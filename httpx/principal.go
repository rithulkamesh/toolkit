package httpx

// TokenKind records how a request authenticated, so downstream middleware can
// treat credential classes differently (see [RequireLiveKey]).
type TokenKind int

const (
	TokenUnknown TokenKind = iota
	TokenSession           // interactive session (cookie or session bearer token)
	TokenLiveKey           // production API key
	TokenTestKey           // sandbox API key — must not reach irreversible actions
)

func (k TokenKind) String() string {
	switch k {
	case TokenSession:
		return "session"
	case TokenLiveKey:
		return "live_key"
	case TokenTestKey:
		return "test_key"
	default:
		return "unknown"
	}
}

// Principal is the authenticated caller. A [Resolver] builds it; middleware and
// handlers read it back with [PrincipalFrom].
type Principal struct {
	Subject   string            // stable user or service id
	TenantID  string            // org / workspace id ("" if not multi-tenant)
	Scopes    []string          // granted permissions, matched exactly
	TokenKind TokenKind         // credential class
	Extra     map[string]string // free-form attributes for custom [Gate]s
}

// HasScope reports whether the principal holds the exact scope.
//
// There is intentionally no wildcard or superuser scope. A single magic scope
// that unlocks every endpoint is a privilege-escalation footgun: roles should
// carry the explicit scopes they need.
func (p *Principal) HasScope(scope string) bool {
	for _, s := range p.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}
