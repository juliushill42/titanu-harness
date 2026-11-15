package harness

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestLittleBitAPIAndLedger(t *testing.T) {
	root := t.TempDir()
	copyAssets(t, root)
	layerPath := filepath.Join(t.TempDir(), "layer.json")
	fixture := []byte(`{"schema_version":1,"input_dim":2,"output_dim":2,"rank":1,"primary":{"u_sign":"AQ==","v_sign":"AQ==","h":[1,1],"g":[1,1],"l":[1]}}`)
	if err := os.WriteFile(layerPath, fixture, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TITANU_LITTLEBIT_LAYER", layerPath)
	a, err := New(root, "", "")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()
	client, token := srv.Client(), a.TokenHex()

	status, body := request(t, client, "GET", srv.URL+"/api/littlebit/status", "", nil)
	expect(t, status, 401, body)
	status, body = request(t, client, "GET", srv.URL+"/api/littlebit/status", token, nil)
	expect(t, status, 200, body)
	if payload(t, body)["configured"] != true {
		t.Fatalf("LittleBit not configured: %s", body)
	}
	status, body = request(t, client, "POST", srv.URL+"/api/littlebit/apply", token, map[string]any{"input": []float64{3, 5}})
	expect(t, status, 200, body)
	result := payload(t, body)
	output, ok := result["output"].([]any)
	if !ok || len(output) != 2 || output[0].(float64) != -2 || output[1].(float64) != 2 {
		t.Fatalf("wrong LittleBit output: %s", body)
	}
	status, body = request(t, client, "GET", srv.URL+"/api/verify", token, nil)
	expect(t, status, 200, body)
	if payload(t, body)["verified"] != true {
		t.Fatalf("LittleBit event not included in valid ledger: %s", body)
	}
	status, body = request(t, client, "GET", srv.URL+"/api/events", token, nil)
	expect(t, status, 200, body)
	if !contains(body, `"kind":"littlebit"`) || !contains(body, `"tool":"littlebit.apply"`) {
		t.Fatalf("LittleBit execution evidence missing: %s", body)
	}
}
