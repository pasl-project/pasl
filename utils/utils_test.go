package utils

import (
	"testing"
)

func TestMax(t *testing.T) {
	if MaxInt32(0, 1) != 1 {
		t.FailNow()
	}
	if MinInt32(0, 1) != 0 {
		t.FailNow()
	}
	if MaxInt64(0, 1) != 1 {
		t.FailNow()
	}
	if MinInt64(0, 1) != 0 {
		t.FailNow()
	}
	if MaxUint32(0, 1) != 1 {
		t.FailNow()
	}
	if MinUint32(0, 1) != 0 {
		t.FailNow()
	}
	if MaxUint64(0, 1) != 1 {
		t.FailNow()
	}
}
