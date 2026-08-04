package graph

import "testing"

func TestDecodeCreatedWpIDParsesObjectData(t *testing.T) {
	got, err := decodeCreatedWpID(`{"success":true,"data":{"id":28488,"wptypeid":3,"sqxx":[]}}`)
	if err != nil {
		t.Fatalf("decodeCreatedWpID() error = %v", err)
	}
	if got != 28488 {
		t.Fatalf("decodeCreatedWpID() = %d, want 28488", got)
	}
}
