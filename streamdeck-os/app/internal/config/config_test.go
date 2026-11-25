package config

import (
	"os"
	"strings"
	"testing"
)

func TestExampleConfigIsValid(t *testing.T) {
	c, err := parse(mustRead(t, "../../../config/config.example.json"))
	if err != nil {
		t.Fatal(err)
	}
	if c.StartPage != "home" || len(c.Pages) != 2 {
		t.Fatalf("unexpected: %+v", c)
	}
	if b, ok := c.Button("home", 1); !ok || b.Label != "Party" {
		t.Fatalf("button lookup failed: %+v", b)
	}
}

func TestValidationCollectsErrors(t *testing.T) {
	_, err := Parse([]byte(`{
	  "pages": {"home": [
	    {"key": 0, "color": "red", "actions": [{"type": "govee_scene", "device": "x"}]},
	    {"key": 0, "actions": [{"type": "page", "page": "nope"}, {"type": "bogus"}]},
	    {"key": 1, "actions": [{"type": "spotify_playlist", "playlist": "spotify:playlist:x"}]}
	  ]}
	}`))
	if err == nil {
		t.Fatal("expected errors")
	}
	for _, want := range []string{"#rrggbb", "needs a scene", "used twice", `page "nope"`, "unknown action", "Govee API key missing", "Spotify login missing"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in:\n%v", want, err)
		}
	}
}

func TestUnknownFieldRejected(t *testing.T) {
	_, err := Parse([]byte(`{"pages":{"home":[]},"brightnes":5}`))
	if err == nil || !strings.Contains(err.Error(), "brightnes") {
		t.Fatalf("want unknown field error, got %v", err)
	}
}

func TestSecretsMerge(t *testing.T) {
	dir := t.TempDir()
	sp := dir + "/secrets.json"
	_, err := LoadAll("../../../config/config.example.json", sp)
	if err == nil || !strings.Contains(err.Error(), "run the installer") {
		t.Fatalf("want missing-secrets error, got %v", err)
	}
	s := &Secrets{Govee: GoveeConfig{APIKey: "g"}, Spotify: SpotifyConfig{ClientID: "i", ClientSecret: "s", RefreshToken: "r", DefaultDevice: "Den"}}
	if err := SaveSecrets(sp, s); err != nil {
		t.Fatal(err)
	}
	c, err := LoadAll("../../../config/config.example.json", sp)
	if err != nil {
		t.Fatal(err)
	}
	if c.Govee.APIKey != "g" || c.Spotify.DefaultDevice != "Den" {
		t.Fatalf("not merged: %+v", c)
	}
}

func mustRead(t *testing.T, p string) []byte {
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
