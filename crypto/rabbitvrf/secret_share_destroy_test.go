package rabbitvrf

import "testing"

func TestSecretShareDestroy(t *testing.T) {
	var encoded [SecretKeySize]byte
	encoded[len(encoded)-1] = 7
	share, err := SecretShareFromBytes(1, encoded[:])
	if err != nil {
		t.Fatal(err)
	}
	share.Destroy()
	share.Destroy()
	if share.ID() != 0 {
		t.Fatal("destroy retained identity")
	}
	if _, err := share.Bytes(); err == nil {
		t.Fatal("destroyed share serialized")
	}
	if _, err := share.PublicKey(); err == nil {
		t.Fatal("destroyed share still usable")
	}
}
