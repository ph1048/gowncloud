package identity

import (
	"crypto/ecdsa"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/dgrijalva/jwt-go"
)

var jwtPubKey *ecdsa.PublicKey

const (
	iyoPubKey = `-----BEGIN PUBLIC KEY-----
MHYwEAYHKoZIzj0CAQYFK4EEACIDYgAES5X8XrfKdx9gYayFITc89wad4usrk0n2
7MjiGYvqalizeSWTHEpnd7oea9IQ8T5oJjMVH5cc0H5tFSKilFFeh//wngxIyny6
6+Vq5t5B0V0Ehy01+2ceEon2Y0XDkIKv
-----END PUBLIC KEY-----`
)

func init() {
	var err error

	jwtPubKey, err = jwt.ParseECPublicKeyFromPEM([]byte(iyoPubKey))
	if err != nil {
		log.Panicf("failed to parse pub key:%v\n", err)
	}
}

func verifyJWTToken(tokenStr string, clientId string) (*Session, error) {
	// verify token
	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		if token.Method != jwt.SigningMethodES384 {
			return nil, fmt.Errorf("Unexpected signing method: %v", token.Header["alg"])
		}
		return jwtPubKey, nil
	})
	if err != nil {
		return nil, err
	}

	// get claims
	claims, ok := token.Claims.(jwt.MapClaims)
	if !(ok && token.Valid) {
		return nil, fmt.Errorf("invalid token")
	}

	var authorizedAudience bool
	// check if we are the authorized party
	authorizedAudience = claims["azp"].(string) == clientId

	if !authorizedAudience {
		//Check if we are the intended audience
		for _, audienceclaim := range claims["aud"].([]interface{}) {
			if authorizedAudience = audienceclaim.(string) == clientId; authorizedAudience {
				break
			}
		}
	}

	if !authorizedAudience {
		return nil, fmt.Errorf("We are not the authorized party or an intended audience - invalid token")
	}

	// check usernames
	username := claims["username"].(string)

	// parse scope claim
	scopes := make([]string, 0, 0)
	rawclaims, ok := claims["scope"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("Error parsing scopes")
	}
	// and store the organizations the user has access to
	orgs := []string{}
	for _, rawclaim := range rawclaims {
		scope, ok := rawclaim.(string)
		if !ok {
			return nil, fmt.Errorf("Error parsing scope")
		}
		scopes = append(scopes, scope)
		if strings.HasPrefix(scope, "user:memberof:") {
			orgs = append(orgs, strings.TrimPrefix(scope, "user:memberof:"))
		}
	}

	return &Session{
		Username:      username,
		Expires:       time.Now().Add(time.Second * time.Duration(int64(claims["exp"].(float64)))),
		Token:         token,
		Organizations: orgs,
	}, nil
}

func getJWTToken(code string, clientID string, clientSecret string, r *http.Request) (string, error) {
	// build request
	hc := http.Client{}
	req, err := http.NewRequest("POST", "https://itsyou.online/v1/oauth/access_token", nil)
	if err != nil {
		return "", err
	}
	q := req.URL.Query()
	q.Add("client_id", clientID)
	q.Add("client_secret", clientSecret)
	q.Add("code", code)
	//TODO: make this request dependent
	q.Add("redirect_uri", "http://"+r.Host+"/oauth/callback")
	q.Add("response_type", "id_token")
	q.Add("scope", "user:memberof:"+clientID)
	q.Add("state", "STATE")
	req.URL.RawQuery = q.Encode()
	// do request
	resp, err := hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	return string(body), err
}
