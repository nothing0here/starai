package main

import (
	"strings"
	"testing"
)

func TestBuildVirtualTryOnPromptPreservesReferenceRoles(t *testing.T) {
	prompt := buildVirtualTryOnPrompt("tops", "flat-lay", "open the jacket")
	for _, want := range []string{"Reference image 1", "Reference image 2", "Replace only the upper-body garment", "flat-lay", "open the jacket", "person identity or garment fidelity"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q: %s", want, prompt)
		}
	}
}

func TestBuildVirtualTryOnPromptModelPhotoDoesNotTransferIdentity(t *testing.T) {
	prompt := buildVirtualTryOnPrompt("bottoms", "model", "")
	if !strings.Contains(prompt, "never the second model's identity") || !strings.Contains(prompt, "Replace only the lower-body garment") {
		t.Fatalf("unexpected prompt: %s", prompt)
	}
}
