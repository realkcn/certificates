package ca

import (
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/smallstep/cli/jose"
)

func getTestProvisioner(t *testing.T, url string) *Provisioner {
	jwk, err := jose.ParseKey("testdata/secrets/ott_mariano_priv.jwk", jose.WithPassword([]byte("password")))
	if err != nil {
		t.Fatal(err)
	}
	return &Provisioner{
		name:          "mariano",
		kid:           "FLIV7q23CXHrg75J2OSbvzwKJJqoxCYixjmsJirneOg",
		caURL:         url,
		caRoot:        "testdata/secrets/root_ca.crt",
		jwk:           jwk,
		tokenLifetime: 5 * time.Minute,
	}
}

func TestNewProvisioner(t *testing.T) {
	value := os.Getenv("STEPPATH")
	defer os.Setenv("STEPPATH", value)
	os.Setenv("STEPPATH", "testdata")

	ca := startCATestServer()
	defer ca.Close()

	want := getTestProvisioner(t, ca.URL)
	wantByKid := getTestProvisioner(t, ca.URL)
	wantByKid.name = ""
	type args struct {
		name     string
		kid      string
		caURL    string
		caRoot   string
		password []byte
	}
	tests := []struct {
		name    string
		args    args
		want    *Provisioner
		wantErr bool
	}{
		{"ok", args{want.name, want.kid, want.caURL, want.caRoot, []byte("password")}, want, false},
		{"ok-by-kid", args{"", want.kid, want.caURL, want.caRoot, []byte("password")}, wantByKid, false},
		{"ok-by-name", args{want.name, "", want.caURL, want.caRoot, []byte("password")}, want, false},
		{"fail-by-kid", args{want.name, "bad-kid", want.caURL, want.caRoot, []byte("password")}, nil, true},
		{"fail-by-name", args{"bad-name", "", want.caURL, want.caRoot, []byte("password")}, nil, true},
		{"fail-by-password", args{"", want.kid, want.caURL, want.caRoot, []byte("bad-password")}, nil, true},
		{"fail-by-password", args{want.name, "", want.caURL, want.caRoot, []byte("bad-password")}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewProvisioner(tt.args.name, tt.args.kid, tt.args.caURL, tt.args.caRoot, tt.args.password)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewProvisioner() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewProvisioner() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProvisioner_Getters(t *testing.T) {
	p := getTestProvisioner(t, "https://127.0.0.1:9000")
	if got := p.Name(); got != p.name {
		t.Errorf("Provisioner.Name() = %v, want %v", got, p.name)
	}
	if got := p.Kid(); got != p.kid {
		t.Errorf("Provisioner.Kid() = %v, want %v", got, p.kid)
	}
}

func TestProvisioner_Token(t *testing.T) {
	p := getTestProvisioner(t, "https://127.0.0.1:9000")
	sha := "ef742f95dc0d8aa82d3cca4017af6dac3fce84290344159891952d18c53eefe7"

	type fields struct {
		name          string
		kid           string
		caURL         string
		caRoot        string
		jwk           *jose.JSONWebKey
		tokenLifetime time.Duration
	}
	type args struct {
		subject string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{"ok", fields{p.name, p.kid, p.caURL, p.caRoot, p.jwk, p.tokenLifetime}, args{"subject"}, false},
		{"fail-no-subject", fields{p.name, p.kid, p.caURL, p.caRoot, p.jwk, p.tokenLifetime}, args{""}, true},
		{"fail-no-key", fields{p.name, p.kid, p.caURL, p.caRoot, &jose.JSONWebKey{}, p.tokenLifetime}, args{"subject"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Provisioner{
				name:          tt.fields.name,
				kid:           tt.fields.kid,
				caURL:         tt.fields.caURL,
				caRoot:        tt.fields.caRoot,
				jwk:           tt.fields.jwk,
				tokenLifetime: tt.fields.tokenLifetime,
			}
			got, err := p.Token(tt.args.subject)
			if (err != nil) != tt.wantErr {
				t.Errorf("Provisioner.Token() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr == false {
				jwt, err := jose.ParseSigned(got)
				if err != nil {
					t.Error(err)
					return
				}
				var claims jose.Claims
				if err := jwt.Claims(tt.fields.jwk.Public(), &claims); err != nil {
					t.Error(err)
					return
				}
				if err := claims.ValidateWithLeeway(jose.Expected{
					Audience: []string{tt.fields.caURL + "/1.0/sign"},
					Issuer:   tt.fields.name,
					Subject:  tt.args.subject,
					Time:     time.Now().UTC(),
				}, time.Minute); err != nil {
					t.Error(err)
					return
				}
				lifetime := claims.Expiry.Time().Sub(claims.NotBefore.Time())
				if lifetime != tt.fields.tokenLifetime {
					t.Errorf("Claims token life time = %s, want %s", lifetime, tt.fields.tokenLifetime)
				}
				allClaims := make(map[string]interface{})
				if err := jwt.Claims(tt.fields.jwk.Public(), &allClaims); err != nil {
					t.Error(err)
					return
				}
				if v, ok := allClaims["sha"].(string); !ok || v != sha {
					t.Errorf("Claim sha = %s, want %s", v, sha)
				}
				if v, ok := allClaims["sans"].([]interface{}); !ok || !reflect.DeepEqual(v, []interface{}{tt.args.subject}) {
					t.Errorf("Claim sans = %s, want %s", v, []interface{}{tt.args.subject})
				}
				if v, ok := allClaims["jti"].(string); !ok || v == "" {
					t.Errorf("Claim jti = %s, want not blank", v)
				}
			}
		})
	}
}
