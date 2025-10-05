package httputil

import (
	"errors"
	"testing"
	"time"

	jwtgo "github.com/golang-jwt/jwt/v5"
)

// TestClaims implements TokenParser for testing
type TestClaims struct {
	*jwtgo.RegisteredClaims
	UserID string `json:"user_id"`
	Role   string `json:"role"`
}

func (c *TestClaims) ParseToken(token *JwtToken) error {
	if !token.Valid {
		return errors.New("token is invalid")
	}
	return nil
}

// TestClaimsWithError implements TokenParser but returns an error
type TestClaimsWithError struct {
	*jwtgo.RegisteredClaims
}

func (c *TestClaimsWithError) ParseToken(token *JwtToken) error {
	return errors.New("custom parse error")
}

// TestClaimsWithoutParser does not implement TokenParser
type TestClaimsWithoutParser struct {
	*jwtgo.RegisteredClaims
	Data string `json:"data"`
}

func TestJwtNew(t *testing.T) {
	secretKey := "test-secret-key"
	jwt := New(secretKey)
	
	if jwt == nil {
		t.Error("New() returned nil")
	}
	
	if string(jwt.secret) != secretKey {
		t.Errorf("New() secret = %v, want %v", string(jwt.secret), secretKey)
	}
}

func TestJwtEncodeAndDecode(t *testing.T) {
	jwt := New("test-secret")
	
	// Create test claims
	claims := &TestClaims{
		RegisteredClaims: &jwtgo.RegisteredClaims{
			ExpiresAt: jwtgo.NewNumericDate(time.Now().Add(1 * time.Hour)),
			IssuedAt:  jwtgo.NewNumericDate(time.Now()),
			Subject:   "test-user",
		},
		UserID: "12345",
		Role:   "admin",
	}
	
	// Test encoding
	token, err := jwt.Encode(claims)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	
	if token == "" {
		t.Error("Encode() returned empty token")
	}
	
	// Test decoding
	decodedClaims := &TestClaims{RegisteredClaims: &jwtgo.RegisteredClaims{}}
	err = jwt.Decode(token, decodedClaims)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	
	// Verify claims
	if decodedClaims.UserID != claims.UserID {
		t.Errorf("Decoded UserID = %v, want %v", decodedClaims.UserID, claims.UserID)
	}
	
	if decodedClaims.Role != claims.Role {
		t.Errorf("Decoded Role = %v, want %v", decodedClaims.Role, claims.Role)
	}
	
	if decodedClaims.Subject != claims.Subject {
		t.Errorf("Decoded Subject = %v, want %v", decodedClaims.Subject, claims.Subject)
	}
}

func TestJwtDecodeWithDifferentSecrets(t *testing.T) {
	jwt1 := New("secret1")
	jwt2 := New("secret2")
	
	claims := &TestClaims{
		RegisteredClaims: &jwtgo.RegisteredClaims{
			ExpiresAt: jwtgo.NewNumericDate(time.Now().Add(1 * time.Hour)),
		},
		UserID: "12345",
	}
	
	// Encode with first secret
	token, err := jwt1.Encode(claims)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	
	// Try to decode with different secret - should fail
	decodedClaims := &TestClaims{RegisteredClaims: &jwtgo.RegisteredClaims{}}
	err = jwt2.Decode(token, decodedClaims)
	if err == nil {
		t.Error("Expected error when decoding with different secret, but got none")
	}
}

func TestJwtDecodeExpiredToken(t *testing.T) {
	jwt := New("test-secret")
	
	// Create expired claims
	claims := &TestClaims{
		RegisteredClaims: &jwtgo.RegisteredClaims{
			ExpiresAt: jwtgo.NewNumericDate(time.Now().Add(-1 * time.Hour)), // Expired 1 hour ago
		},
		UserID: "12345",
	}
	
	token, err := jwt.Encode(claims)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	
	// Try to decode expired token
	decodedClaims := &TestClaims{RegisteredClaims: &jwtgo.RegisteredClaims{}}
	err = jwt.Decode(token, decodedClaims)
	if err == nil {
		t.Error("Expected error when decoding expired token, but got none")
	}
}

func TestJwtDecodeInvalidToken(t *testing.T) {
	jwt := New("test-secret")
	
	invalidTokens := []string{
		"invalid.token.format",
		"",
		"not-a-jwt-token",
		"header.payload", // Missing signature
		"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.invalid-payload.signature",
	}
	
	for _, invalidToken := range invalidTokens {
		t.Run("invalid_token_"+invalidToken, func(t *testing.T) {
			claims := &TestClaims{RegisteredClaims: &jwtgo.RegisteredClaims{}}
			err := jwt.Decode(invalidToken, claims)
			if err == nil {
				t.Errorf("Expected error for invalid token %q, but got none", invalidToken)
			}
		})
	}
}

func TestJwtDecodeWithoutTokenParser(t *testing.T) {
	jwt := New("test-secret")
	
	// Create claims that don't implement TokenParser
	claims := &TestClaimsWithoutParser{
		RegisteredClaims: &jwtgo.RegisteredClaims{
			ExpiresAt: jwtgo.NewNumericDate(time.Now().Add(1 * time.Hour)),
		},
		Data: "test-data",
	}
	
	token, err := jwt.Encode(claims)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	
	// Try to decode with claims that don't implement TokenParser
	decodedClaims := &TestClaimsWithoutParser{RegisteredClaims: &jwtgo.RegisteredClaims{}}
	err = jwt.Decode(token, decodedClaims)
	if err == nil {
		t.Error("Expected error when claims don't implement TokenParser, but got none")
	}
	
	if err.Error() != "claims does not implement TokenParser" {
		t.Errorf("Expected specific error message, got: %v", err)
	}
}

func TestJwtDecodeWithParseError(t *testing.T) {
	jwt := New("test-secret")
	
	// Create claims with TokenParser that returns error
	claims := &TestClaimsWithError{
		RegisteredClaims: &jwtgo.RegisteredClaims{
			ExpiresAt: jwtgo.NewNumericDate(time.Now().Add(1 * time.Hour)),
		},
	}
	
	token, err := jwt.Encode(claims)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	
	// Try to decode - should fail in ParseToken
	decodedClaims := &TestClaimsWithError{RegisteredClaims: &jwtgo.RegisteredClaims{}}
	err = jwt.Decode(token, decodedClaims)
	if err == nil {
		t.Error("Expected error from ParseToken, but got none")
	}
	
	if err.Error() != "custom parse error" {
		t.Errorf("Expected custom parse error, got: %v", err)
	}
}

func TestJwtMapClaims(t *testing.T) {
	jwt := New("test-secret")
	
	// Test with MapClaims that implements TokenParser
	claims := JwtMapClaims{
		"user_id": "12345",
		"role":    "admin",
		"exp":     time.Now().Add(1 * time.Hour).Unix(),
	}
	
	// For MapClaims to work with our Decode method, we need to wrap it
	// This shows the limitation of the current design
	token, err := jwt.Encode(claims)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	
	if token == "" {
		t.Error("Encode() returned empty token for MapClaims")
	}
	
	// Note: We can't test Decode with MapClaims because it doesn't implement TokenParser
	// This is by design as shown in the current implementation
}

func TestJwtAliases(t *testing.T) {
	// Test that type aliases are properly defined
	var token *JwtToken
	var claims JwtClaims
	var regClaims *JwtRegisteredClaims
	var mapClaims JwtMapClaims
	
	// These should compile without errors
	_ = token
	_ = claims
	_ = regClaims
	_ = mapClaims
	
	// Test function aliases
	if JwtParseRSAPrivateKeyFromPEM == nil {
		t.Error("JwtParseRSAPrivateKeyFromPEM alias not set")
	}
	
	if JwtNewWithClaims == nil {
		t.Error("JwtNewWithClaims alias not set")
	}
	
	if JwtSigningMethodRS256 == nil {
		t.Error("JwtSigningMethodRS256 alias not set")
	}
	
	if JwtNewNumericDate == nil {
		t.Error("JwtNewNumericDate alias not set")
	}
}

func TestJwtEncodeDecode_RealWorldScenario(t *testing.T) {
	// Test a more realistic scenario
	jwt := New("my-app-secret-key-2024")
	
	// Create realistic claims
	issuedAt := time.Now()
	expiresAt := issuedAt.Add(24 * time.Hour)
	
	claims := &TestClaims{
		RegisteredClaims: &jwtgo.RegisteredClaims{
			Issuer:    "my-app",
			Subject:   "user@example.com",
			Audience:  []string{"web-app", "mobile-app"},
			ExpiresAt: jwtgo.NewNumericDate(expiresAt),
			NotBefore: jwtgo.NewNumericDate(issuedAt),
			IssuedAt:  jwtgo.NewNumericDate(issuedAt),
			ID:        "token-id-123",
		},
		UserID: "user-12345",
		Role:   "premium",
	}
	
	// Encode
	token, err := jwt.Encode(claims)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	
	// Decode
	decodedClaims := &TestClaims{RegisteredClaims: &jwtgo.RegisteredClaims{}}
	err = jwt.Decode(token, decodedClaims)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	
	// Verify all fields
	if decodedClaims.Issuer != claims.Issuer {
		t.Errorf("Issuer mismatch: got %v, want %v", decodedClaims.Issuer, claims.Issuer)
	}
	
	if decodedClaims.Subject != claims.Subject {
		t.Errorf("Subject mismatch: got %v, want %v", decodedClaims.Subject, claims.Subject)
	}
	
	if len(decodedClaims.Audience) != len(claims.Audience) {
		t.Errorf("Audience length mismatch: got %v, want %v", len(decodedClaims.Audience), len(claims.Audience))
	}
	
	if decodedClaims.ID != claims.ID {
		t.Errorf("ID mismatch: got %v, want %v", decodedClaims.ID, claims.ID)
	}
	
	if decodedClaims.UserID != claims.UserID {
		t.Errorf("UserID mismatch: got %v, want %v", decodedClaims.UserID, claims.UserID)
	}
	
	if decodedClaims.Role != claims.Role {
		t.Errorf("Role mismatch: got %v, want %v", decodedClaims.Role, claims.Role)
	}
}