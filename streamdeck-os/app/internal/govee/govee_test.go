package govee

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestApplyScene(t *testing.T) {
	var control map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Govee-API-Key") != "k" {
			w.WriteHeader(401)
			return
		}
		body, _ := io.ReadAll(r.Body)
		switch r.URL.Path {
		case "/user/devices":
			io.WriteString(w, `{"code":200,"message":"success","data":[{"sku":"H6199","device":"AA:BB","deviceName":"TV Backlight","type":"devices.types.light","capabilities":[]}]}`)
		case "/device/scenes":
			io.WriteString(w, `{"requestId":"x","msg":"success","code":200,"payload":{"sku":"H6199","device":"AA:BB","capabilities":[{"type":"devices.capabilities.dynamic_scene","instance":"lightScene","parameters":{"dataType":"ENUM","options":[{"name":"Sunrise","value":{"paramId":4280,"id":3853}},{"name":"Movie","value":{"paramId":11,"id":22}}]}}]}}`)
		case "/device/control":
			json.Unmarshal(body, &control)
			io.WriteString(w, `{"requestId":"x","msg":"success","code":200}`)
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	c := New("k")
	c.BaseURL = srv.URL
	if err := c.ApplyScene(context.Background(), "tv backlight", "movie", false); err != nil {
		t.Fatal(err)
	}
	p := control["payload"].(map[string]any)
	capab := p["capability"].(map[string]any)
	v := capab["value"].(map[string]any)
	if p["sku"] != "H6199" || p["device"] != "AA:BB" || capab["instance"] != "lightScene" ||
		capab["type"] != "devices.capabilities.dynamic_scene" || v["id"] != float64(22) || v["paramId"] != float64(11) {
		t.Fatalf("bad control body: %v", control)
	}
	if control["requestId"] == "" {
		t.Fatal("missing requestId")
	}

	err := c.ApplyScene(context.Background(), "TV Backlight", "Nope", false)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("want not found, got %v", err)
	}
	err = c.Power(context.Background(), "Kitchen", true)
	if err == nil || !strings.Contains(err.Error(), `"TV Backlight"`) {
		t.Fatalf("want device list in error, got %v", err)
	}
}

func TestErrorCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"code":400,"msg":"devices not exist"}`)
	}))
	defer srv.Close()
	c := New("k")
	c.BaseURL = srv.URL
	if _, err := c.Devices(context.Background()); err == nil || !strings.Contains(err.Error(), "devices not exist") {
		t.Fatalf("got %v", err)
	}
}

func TestLightsLeavesOutGroups(t *testing.T) {
	devs := []Device{
		{SKU: "H6199", Name: "TV Backlight", Type: "devices.types.light"},
		{SKU: "SameModeGroup", Name: "Living Room Lights", Type: "devices.types.group"},
		{SKU: "BaseGroup", Name: "All"},
		{SKU: "H6076", Name: "Floor Lamp", Type: "devices.types.light"},
	}
	got := Lights(devs)
	if len(got) != 2 || got[0].Name != "TV Backlight" || got[1].Name != "Floor Lamp" {
		t.Fatalf("got %+v", got)
	}
}
