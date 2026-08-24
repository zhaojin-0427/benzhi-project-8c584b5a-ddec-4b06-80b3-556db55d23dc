package main

import "testing"

func TestAddressFromEnvironment(t *testing.T) {
	address, err := addressFromEnvironment("")
	if err != nil || address != defaultAddress {
		t.Fatalf("unexpected default: %s %v", address, err)
	}
	address, err = addressFromEnvironment("19123")
	if err != nil || address != "127.0.0.1:19123" {
		t.Fatalf("unexpected PORT address: %s %v", address, err)
	}
	if _, err := addressFromEnvironment("invalid"); err == nil {
		t.Fatal("expected invalid PORT error")
	}
}

func TestValidateAddressRejectsNonLoopback(t *testing.T) {
	if err := validateAddress("127.0.0.1:19081"); err != nil {
		t.Fatal(err)
	}
	if err := validateAddress("0.0.0.0:19081"); err == nil {
		t.Fatal("expected non-loopback rejection")
	}
}
