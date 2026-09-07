// Package awssm reads secrets from AWS Secrets Manager and SSM Parameter
// Store, exposing each as a [secretsx.Provider].
//
//	sm, err := awssm.NewSecretsManager(ctx)
//	store := secretsx.Chain{sm, secretsx.Env{Prefix: "APP_"}}
//	pw, err := store.Get(ctx, "prod/db#password")
//
// A key is "name" for the raw secret, or "name#field" (or "name#a/b/c") to
// parse the secret value as JSON and pull one field out of it.
package awssm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	smtypes "github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"

	"github.com/rithulkamesh/toolkit/secretsx"
)

// SecretsManagerAPI is the subset of the Secrets Manager client this package
// uses. The real *secretsmanager.Client satisfies it; tests supply a fake.
type SecretsManagerAPI interface {
	GetSecretValue(context.Context, *secretsmanager.GetSecretValueInput, ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error)
}

// ParameterStoreAPI is the subset of the SSM client this package uses.
type ParameterStoreAPI interface {
	GetParameter(context.Context, *ssm.GetParameterInput, ...func(*ssm.Options)) (*ssm.GetParameterOutput, error)
}

// SecretsManager is a [secretsx.Provider] backed by AWS Secrets Manager.
type SecretsManager struct {
	API    SecretsManagerAPI
	Prefix string // prepended to every key's secret name
}

// ParameterStore is a [secretsx.Provider] backed by SSM Parameter Store. It
// always requests WithDecryption, so SecureString parameters come back plain.
type ParameterStore struct {
	API    ParameterStoreAPI
	Prefix string // prepended to every key's parameter name
}

// NewSecretsManager builds a [SecretsManager] from the default AWS config
// chain (env, shared config, IAM role, ...).
func NewSecretsManager(ctx context.Context, optFns ...func(*config.LoadOptions) error) (*SecretsManager, error) {
	cfg, err := config.LoadDefaultConfig(ctx, optFns...)
	if err != nil {
		return nil, fmt.Errorf("awssm: load AWS config: %w", err)
	}
	return &SecretsManager{API: secretsmanager.NewFromConfig(cfg)}, nil
}

// NewParameterStore builds a [ParameterStore] from the default AWS config chain.
func NewParameterStore(ctx context.Context, optFns ...func(*config.LoadOptions) error) (*ParameterStore, error) {
	cfg, err := config.LoadDefaultConfig(ctx, optFns...)
	if err != nil {
		return nil, fmt.Errorf("awssm: load AWS config: %w", err)
	}
	return &ParameterStore{API: ssm.NewFromConfig(cfg)}, nil
}

// Get implements [secretsx.Provider].
func (s *SecretsManager) Get(ctx context.Context, key string) (string, error) {
	name, field, _ := strings.Cut(key, "#")
	out, err := s.API.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(s.Prefix + name),
	})
	if err != nil {
		var nf *smtypes.ResourceNotFoundException
		if errors.As(err, &nf) {
			return "", fmt.Errorf("awssm: secret %q: %w", name, secretsx.ErrNotFound)
		}
		return "", fmt.Errorf("awssm: GetSecretValue %q: %w", name, err)
	}

	var value string
	switch {
	case out.SecretString != nil:
		value = *out.SecretString
	case out.SecretBinary != nil:
		value = string(out.SecretBinary)
	default:
		return "", fmt.Errorf("awssm: secret %q: %w", name, secretsx.ErrNotFound)
	}
	return extractField(value, field, name)
}

// Get implements [secretsx.Provider].
func (p *ParameterStore) Get(ctx context.Context, key string) (string, error) {
	name, field, _ := strings.Cut(key, "#")
	out, err := p.API.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           aws.String(p.Prefix + name),
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		var nf *ssmtypes.ParameterNotFound
		if errors.As(err, &nf) {
			return "", fmt.Errorf("awssm: parameter %q: %w", name, secretsx.ErrNotFound)
		}
		return "", fmt.Errorf("awssm: GetParameter %q: %w", name, err)
	}
	if out.Parameter == nil || out.Parameter.Value == nil {
		return "", fmt.Errorf("awssm: parameter %q: %w", name, secretsx.ErrNotFound)
	}
	return extractField(*out.Parameter.Value, field, name)
}

// extractField returns raw when field is empty; otherwise it parses raw as a
// JSON object and walks "a/b/c" into it.
func extractField(raw, field, name string) (string, error) {
	if field == "" {
		return raw, nil
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return "", fmt.Errorf("awssm: secret %q is not a JSON object, cannot read field %q: %w", name, field, err)
	}
	var cur any = obj
	for _, seg := range strings.Split(field, "/") {
		m, ok := cur.(map[string]any)
		if !ok {
			return "", fmt.Errorf("awssm: secret %q has no field %q: %w", name, field, secretsx.ErrNotFound)
		}
		cur, ok = m[seg]
		if !ok {
			return "", fmt.Errorf("awssm: secret %q has no field %q: %w", name, field, secretsx.ErrNotFound)
		}
	}
	if s, ok := cur.(string); ok {
		return s, nil
	}
	enc, err := json.Marshal(cur)
	if err != nil {
		return "", err
	}
	return string(enc), nil
}

func init() {
	// "aws-sm:" and "aws-sm:region" — everything else from the AWS config chain.
	secretsx.Register("aws-sm", func(rest string) (secretsx.Provider, error) {
		return newFromDSN(rest, func(cfg aws.Config) secretsx.Provider {
			return &SecretsManager{API: secretsmanager.NewFromConfig(cfg)}
		})
	})
	secretsx.Register("aws-ssm", func(rest string) (secretsx.Provider, error) {
		return newFromDSN(rest, func(cfg aws.Config) secretsx.Provider {
			return &ParameterStore{API: ssm.NewFromConfig(cfg)}
		})
	})
}

func newFromDSN(region string, build func(aws.Config) secretsx.Provider) (secretsx.Provider, error) {
	var optFns []func(*config.LoadOptions) error
	if region != "" {
		optFns = append(optFns, config.WithRegion(region))
	}
	cfg, err := config.LoadDefaultConfig(context.Background(), optFns...)
	if err != nil {
		return nil, fmt.Errorf("awssm: load AWS config: %w", err)
	}
	return build(cfg), nil
}
