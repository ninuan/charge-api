package api

import (
	"strconv"
	"testing"
)

func TestStreamLimitsEnforceUserAndGlobalCaps(t *testing.T) {
	var limits streamLimits
	for i := 0; i < maxStreamsPerUser; i++ {
		if !limits.acquire("user-1") {
			t.Fatalf("user stream %d was rejected", i)
		}
	}
	if limits.acquire("user-1") {
		t.Fatal("per-user stream cap was not enforced")
	}
	limits.release("user-1")
	if !limits.acquire("user-1") {
		t.Fatal("released user stream slot was not reusable")
	}
	for i := 0; i < maxStreamsTotal-maxStreamsPerUser; i++ {
		if !limits.acquire("user-" + strconv.Itoa(i+2)) {
			t.Fatalf("global stream %d was rejected", i)
		}
	}
	if limits.acquire("overflow") {
		t.Fatal("global stream cap was not enforced")
	}
}
