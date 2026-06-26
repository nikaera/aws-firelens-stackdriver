package main

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestWIFConfigMissingRequiredOptionsOrder(t *testing.T) {
	t.Parallel()

	got := (wifConfig{AWSRegion: "ap-northeast-1"}).missingRequiredOptions()
	want := []string{
		optionProjectNumber,
		optionPoolID,
		optionProviderID,
		optionGoogleServiceAccount,
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("missingRequiredOptions mismatch (-want +got):\n%s", diff)
	}
}
