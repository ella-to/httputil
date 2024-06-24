package httputil

import (
	"errors"

	jwtgo "github.com/golang-jwt/jwt/v5"
)

type (
	JwtToken            = jwtgo.Token
	JwtClaims           = jwtgo.Claims
	JwtRegisteredClaims = jwtgo.RegisteredClaims
	JwtMapClaims        = jwtgo.MapClaims
)

var (
	JwtParseRSAPrivateKeyFromPEM = jwtgo.ParseRSAPrivateKeyFromPEM
	JwtNewWithClaims             = jwtgo.NewWithClaims
	JwtSigningMethodRS256        = jwtgo.SigningMethodRS256
	JwtNewNumericDate            = jwtgo.NewNumericDate
)

type TokenParser interface {
	ParseToken(token *JwtToken) error
}

type Jwt struct {
	secret []byte
}

func (j *Jwt) Encode(claims JwtClaims) (string, error) {
	return encode(j.secret, claims)
}

func (j *Jwt) Decode(jwt string, claims JwtClaims) error {
	token, err := decode(j.secret, jwt, claims)
	if err != nil {
		return err
	}

	tokenParser, ok := claims.(TokenParser)
	if !ok {
		return errors.New("claims does not implement TokenParser")
	}

	return tokenParser.ParseToken(token)
}

func New(secretKey string) *Jwt {
	return &Jwt{
		secret: []byte(secretKey),
	}
}

func encode(secretKey []byte, claims JwtClaims) (string, error) {
	token := jwtgo.NewWithClaims(jwtgo.SigningMethodHS256, claims)

	tok, err := token.SignedString(secretKey)
	if err != nil {
		return "", err
	}

	return tok, nil
}

func decode(secretKey []byte, jwtValue string, claims JwtClaims) (*JwtToken, error) {
	return jwtgo.ParseWithClaims(jwtValue, claims, func(token *JwtToken) (interface{}, error) {
		if _, ok := token.Method.(*jwtgo.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return secretKey, nil
	})
}
