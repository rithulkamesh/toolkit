package awssm

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	smtypes "github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"

	"github.com/rithulkamesh/toolkit/secretsx"
)

type fakeSM func(id string) (*secretsmanager.GetSecretValueOutput, error)

func (f fakeSM) GetSecretValue(_ context.Context, in *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
	return f(aws.ToString(in.SecretId))
}

func TestSecretsManagerGet(t *testing.T) {
	api := fakeSM(func(id string) (*secretsmanager.GetSecretValueOutput, error) {
		switch id {
		case "prod/api-key":
			return &secretsmanager.GetSecretValueOutput{SecretString: aws.String("raw-key")}, nil
		case "prod/db":
			return &secretsmanager.GetSecretValueOutput{SecretString: aws.String(`{"username":"u","password":"p","opts":{"ssl":"require"}}`)}, nil
		default:
			return nil, &smtypes.ResourceNotFoundException{}
		}
	})
	s := &SecretsManager{API: api}
	ctx := context.Background()

	if v, err := s.Get(ctx, "prod/api-key"); err != nil || v != "raw-key" {
		t.Fatalf("raw secret = %q, %v", v, err)
	}
	if v, err := s.Get(ctx, "prod/db#password"); err != nil || v != "p" {
		t.Fatalf("json field = %q, %v", v, err)
	}
	if v, err := s.Get(ctx, "prod/db#opts/ssl"); err != nil || v != "require" {
		t.Fatalf("nested json field = %q, %v", v, err)
	}
	if _, err := s.Get(ctx, "prod/db#missing"); !errors.Is(err, secretsx.ErrNotFound) {
		t.Fatalf("missing field: want ErrNotFound, got %v", err)
	}
	if _, err := s.Get(ctx, "nope"); !errors.Is(err, secretsx.ErrNotFound) {
		t.Fatalf("missing secret: want ErrNotFound, got %v", err)
	}
}

type fakeSSM func(name string, decrypt bool) (*ssm.GetParameterOutput, error)

func (f fakeSSM) GetParameter(_ context.Context, in *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
	return f(aws.ToString(in.Name), aws.ToBool(in.WithDecryption))
}

func TestParameterStoreGet(t *testing.T) {
	api := fakeSSM(func(name string, decrypt bool) (*ssm.GetParameterOutput, error) {
		if !decrypt {
			t.Error("expected WithDecryption=true")
		}
		if name == "/app/token" {
			return &ssm.GetParameterOutput{Parameter: &ssmtypes.Parameter{Value: aws.String("sekret")}}, nil
		}
		return nil, &ssmtypes.ParameterNotFound{}
	})
	p := &ParameterStore{API: api}

	if v, err := p.Get(context.Background(), "/app/token"); err != nil || v != "sekret" {
		t.Fatalf("param = %q, %v", v, err)
	}
	if _, err := p.Get(context.Background(), "/app/missing"); !errors.Is(err, secretsx.ErrNotFound) {
		t.Fatalf("missing param: want ErrNotFound, got %v", err)
	}
}

func TestPrefix(t *testing.T) {
	var gotID string
	api := fakeSM(func(id string) (*secretsmanager.GetSecretValueOutput, error) {
		gotID = id
		return &secretsmanager.GetSecretValueOutput{SecretString: aws.String("x")}, nil
	})
	s := &SecretsManager{API: api, Prefix: "team-a/"}
	if _, err := s.Get(context.Background(), "db"); err != nil {
		t.Fatal(err)
	}
	if gotID != "team-a/db" {
		t.Fatalf("prefixed id = %q, want team-a/db", gotID)
	}
}
