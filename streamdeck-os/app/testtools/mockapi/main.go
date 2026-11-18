// mockapi impersonates the Govee and Spotify endpoints streamdeckd uses so
// the VM test can run fully offline. Every request is logged as
//
//	MOCKAPI: METHOD /path?query body
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
)

const (
	GoveeKey     = "tEsT-gOvEe-42xq"
	ClientID     = "mockclientid"
	ClientSecret = "mocksecret"
	AuthCode     = "MOCKCODE"
	RefreshToken = "MOCKREFRESH"
	AccessToken  = "MOCKACCESS"
	RedirectURI  = "http://127.0.0.1:8888/callback"
)

var scenes = `{"code":200,"msg":"success","payload":{"capabilities":[{"type":"devices.capabilities.dynamic_scene","instance":"lightScene","parameters":{"dataType":"ENUM","options":[` +
	`{"name":"Movie","value":{"paramId":101,"id":1}},{"name":"Party","value":{"paramId":102,"id":2}},` +
	`{"name":"Sunset","value":{"paramId":103,"id":3}},{"name":"Rainbow","value":{"paramId":104,"id":4}}]}}]}}`

func main() {
	addr := flag.String("listen", ":9999", "listen address")
	flag.Parse()
	log.SetFlags(0)
	log.SetPrefix("MOCKAPI: ")

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		log.Printf("%s %s?%s %s", r.Method, r.URL.Path, r.URL.RawQuery, strings.TrimSpace(string(body)))
		r.Body = io.NopCloser(strings.NewReader(string(body)))
		p := r.URL.Path
		switch {
		case strings.HasPrefix(p, "/govee/"):
			govee(w, r, strings.TrimPrefix(p, "/govee"))
		case p == "/spotify/authorize":
			// Pretend the user logged in and pressed Agree.
			q := r.URL.Query()
			if q.Get("client_id") != ClientID || q.Get("redirect_uri") != RedirectURI {
				http.Error(w, "INVALID_CLIENT: Invalid redirect URI", 400)
				return
			}
			http.Redirect(w, r, RedirectURI+"?code="+AuthCode+"&state="+url.QueryEscape(q.Get("state")), http.StatusFound)
		case p == "/spotify/token":
			token(w, r)
		case strings.HasPrefix(p, "/spotify/api/"):
			player(w, r, strings.TrimPrefix(p, "/spotify/api"))
		default:
			http.NotFound(w, r)
		}
	})
	log.Printf("listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}

func govee(w http.ResponseWriter, r *http.Request, p string) {
	if r.Header.Get("Govee-API-Key") != GoveeKey {
		w.WriteHeader(401)
		io.WriteString(w, `{"code":401,"message":"Invalid API Key"}`)
		return
	}
	switch p {
	case "/user/devices":
		io.WriteString(w, `{"code":200,"message":"success","data":[`+
			`{"sku":"H6199","device":"AA:AA:AA:AA:AA:AA:AA:01","deviceName":"TV Backlight","type":"devices.types.light"},`+
			`{"sku":"H6076","device":"AA:AA:AA:AA:AA:AA:AA:02","deviceName":"Floor Lamp","type":"devices.types.light"}]}`)
	case "/device/scenes":
		io.WriteString(w, scenes)
	case "/device/diy-scenes":
		io.WriteString(w, `{"code":200,"msg":"success","payload":{"capabilities":[]}}`)
	case "/device/control":
		io.WriteString(w, `{"code":200,"msg":"success"}`)
	default:
		w.WriteHeader(404)
	}
}

func token(w http.ResponseWriter, r *http.Request) {
	user, pass, _ := r.BasicAuth()
	_ = r.ParseForm()
	if user != ClientID || pass != ClientSecret {
		w.WriteHeader(400)
		io.WriteString(w, `{"error":"invalid_client"}`)
		return
	}
	switch r.Form.Get("grant_type") {
	case "authorization_code":
		if r.Form.Get("code") == AuthCode && r.Form.Get("redirect_uri") == RedirectURI {
			fmt.Fprintf(w, `{"access_token":%q,"refresh_token":%q,"expires_in":3600}`, AccessToken, RefreshToken)
			return
		}
	case "refresh_token":
		if r.Form.Get("refresh_token") == RefreshToken {
			fmt.Fprintf(w, `{"access_token":%q,"expires_in":3600}`, AccessToken)
			return
		}
	}
	w.WriteHeader(400)
	io.WriteString(w, `{"error":"invalid_grant"}`)
}

func player(w http.ResponseWriter, r *http.Request, p string) {
	if r.Header.Get("Authorization") != "Bearer "+AccessToken {
		w.WriteHeader(401)
		return
	}
	if p == "/me/playlists" {
		io.WriteString(w, `{"items":[`+
			`{"uri":"spotify:playlist:PLDINNER","name":"Dinner Jazz","owner":{"display_name":"Hunter"},"tracks":{"total":42},"images":[{"url":"https://i.scdn.co/x.jpg","width":60}]},`+
			`{"uri":"spotify:playlist:PLWORKOUT","name":"Workout Mix","owner":{"display_name":"Hunter"},"tracks":{"total":30},"images":[]}],"next":null}`)
		return
	}
	if p == "/me/player/devices" {
		io.WriteString(w, `{"devices":[{"id":"dev-living","name":"Living Room","type":"Speaker","is_active":false},`+
			`{"id":"dev-kitchen","name":"Kitchen Echo","type":"Speaker","is_active":false}]}`)
		return
	}
	w.WriteHeader(204)
}
