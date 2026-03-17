package bitflag

import (
	"database/sql/driver"
	"math"
	"testing"
)

type UserFlag int32

const (
	UserFlagUnspecified UserFlag = 0
	UserFlagVerified    UserFlag = 1
	UserFlagVIP         UserFlag = 2
	UserFlagBanned      UserFlag = 3
	UserFlagInternal    UserFlag = 4
	UserFlagFeatureX    UserFlag = 5
)

func TestSetAndHas(t *testing.T) {
	var f Flags[UserFlag]

	f = f.Set(UserFlagVerified)
	f = f.Set(UserFlagVIP)

	if !f.Has(UserFlagVerified) {
		t.Fatal("expected verified to be set")
	}
	if !f.Has(UserFlagVIP) {
		t.Fatal("expected vip to be set")
	}
	if f.Has(UserFlagBanned) {
		t.Fatal("expected banned to be unset")
	}
}

func TestHasAllAndHasAny(t *testing.T) {
	f := New[UserFlag](UserFlagVerified, UserFlagVIP)

	if !f.HasAll(UserFlagVerified, UserFlagVIP) {
		t.Fatal("expected HasAll to be true")
	}
	if f.HasAll(UserFlagVerified, UserFlagBanned) {
		t.Fatal("expected HasAll to be false")
	}
	if !f.HasAny(UserFlagBanned, UserFlagVIP) {
		t.Fatal("expected HasAny to be true")
	}
	if f.HasAny(UserFlagBanned, UserFlagInternal) {
		t.Fatal("expected HasAny to be false")
	}
}

func TestClear(t *testing.T) {
	f := New[UserFlag](UserFlagVerified, UserFlagVIP, UserFlagBanned)
	f = f.Clear(UserFlagVIP)

	if f.Has(UserFlagVIP) {
		t.Fatal("expected vip to be cleared")
	}
	if !f.Has(UserFlagVerified) {
		t.Fatal("expected verified to remain set")
	}
	if !f.Has(UserFlagBanned) {
		t.Fatal("expected banned to remain set")
	}
}

func TestClearAll(t *testing.T) {
	f := New[UserFlag](UserFlagVerified, UserFlagVIP, UserFlagBanned, UserFlagFeatureX)
	f = f.ClearAll(UserFlagVIP, UserFlagFeatureX)

	if f.Has(UserFlagVIP) {
		t.Fatal("expected vip to be cleared")
	}
	if f.Has(UserFlagFeatureX) {
		t.Fatal("expected feature_x to be cleared")
	}
	if !f.Has(UserFlagVerified) || !f.Has(UserFlagBanned) {
		t.Fatal("expected remaining flags to stay set")
	}
}

func TestSetTo(t *testing.T) {
	var f Flags[UserFlag]

	f = f.SetTo(UserFlagFeatureX, true)
	if !f.Has(UserFlagFeatureX) {
		t.Fatal("expected feature_x to be set")
	}

	f = f.SetTo(UserFlagFeatureX, false)
	if f.Has(UserFlagFeatureX) {
		t.Fatal("expected feature_x to be cleared")
	}
}

func TestToggle(t *testing.T) {
	var f Flags[UserFlag]

	f = f.Toggle(UserFlagVerified)
	if !f.Has(UserFlagVerified) {
		t.Fatal("expected verified to be toggled on")
	}

	f = f.Toggle(UserFlagVerified)
	if f.Has(UserFlagVerified) {
		t.Fatal("expected verified to be toggled off")
	}
}

func TestMask(t *testing.T) {
	f := New[UserFlag](UserFlagVerified, UserFlagVIP, UserFlagFeatureX)

	must := MaskOf[UserFlag](UserFlagVerified, UserFlagVIP)
	any := MaskOf[UserFlag](UserFlagBanned, UserFlagFeatureX)

	if !f.Contains(must) {
		t.Fatal("expected Contains to be true")
	}
	if !f.Intersects(any) {
		t.Fatal("expected Intersects to be true")
	}

	notContained := MaskOf[UserFlag](UserFlagVerified, UserFlagBanned)
	if f.Contains(notContained) {
		t.Fatal("expected Contains to be false")
	}
}

func TestInt64RoundTrip(t *testing.T) {
	f := New[UserFlag](UserFlagVerified, UserFlagVIP, UserFlagFeatureX)
	v := f.Int64()

	f2 := FromInt64[UserFlag](v)
	if !f2.HasAll(UserFlagVerified, UserFlagVIP, UserFlagFeatureX) {
		t.Fatal("expected roundtrip flags to match")
	}
}

func TestValue(t *testing.T) {
	f := New[UserFlag](UserFlagVerified, UserFlagVIP)

	v, err := f.Value()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	n, ok := v.(int64)
	if !ok {
		t.Fatalf("expected int64 driver.Value, got %T", v)
	}
	if n != f.Int64() {
		t.Fatalf("expected value %d, got %d", f.Int64(), n)
	}

	var _ driver.Valuer = f
}

func TestScan(t *testing.T) {
	var f Flags[UserFlag]

	if err := f.Scan(int64(3)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !f.Has(UserFlagVerified) || !f.Has(UserFlagVIP) {
		t.Fatal("expected verified and vip to be set from int64(3)")
	}

	if err := f.Scan("5"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !f.Has(UserFlagVerified) || !f.Has(UserFlagBanned) {
		t.Fatal("expected verified and banned to be set from string(5)")
	}
}

func TestValid(t *testing.T) {
	if Valid(UserFlagUnspecified) {
		t.Fatal("0 should be invalid")
	}
	if !Valid(UserFlagVerified) {
		t.Fatal("1 should be valid")
	}
}

func TestLen(t *testing.T) {
	var f Flags[UserFlag]
	if f.Len() != 0 {
		t.Fatalf("expected len=0, got %d", f.Len())
	}

	f = New[UserFlag](UserFlagVerified, UserFlagVIP, UserFlagFeatureX)
	if f.Len() != 3 {
		t.Fatalf("expected len=3, got %d", f.Len())
	}
}

func TestEnums(t *testing.T) {
	f := New[UserFlag](UserFlagFeatureX, UserFlagVerified, UserFlagBanned)

	got := f.Enums()
	want := []UserFlag{UserFlagVerified, UserFlagBanned, UserFlagFeatureX}

	if len(got) != len(want) {
		t.Fatalf("expected len=%d, got %d", len(want), len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected enums[%d]=%d, got %d", i, want[i], got[i])
		}
	}
}

func TestClone(t *testing.T) {
	f1 := New[UserFlag](UserFlagVerified, UserFlagVIP)
	f2 := f1.Clone()

	if f1.Int64() != f2.Int64() {
		t.Fatal("expected clone to have same value")
	}

	f2 = f2.Set(UserFlagBanned)
	if f1.Has(UserFlagBanned) {
		t.Fatal("expected original not to change")
	}
	if !f2.Has(UserFlagBanned) {
		t.Fatal("expected clone to change independently")
	}
}

func TestString(t *testing.T) {
	var zero Flags[UserFlag]
	if zero.String() != "bitflag(0:[])" {
		t.Fatalf("unexpected zero string: %s", zero.String())
	}

	f := New[UserFlag](UserFlagVerified, UserFlagBanned, UserFlagInternal)
	got := f.String()
	want := "bitflag(13:[1,3,4])"

	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestFormat(t *testing.T) {
	f := New[UserFlag](UserFlagVerified, UserFlagBanned, UserFlagInternal)

	got := f.Format(func(v UserFlag) string {
		switch v {
		case UserFlagVerified:
			return "VERIFIED"
		case UserFlagBanned:
			return "BANNED"
		case UserFlagInternal:
			return "INTERNAL"
		default:
			return "UNKNOWN"
		}
	})

	want := "bitflag(13:[VERIFIED,BANNED,INTERNAL])"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestInvalidEnumPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for invalid enum value")
		}
	}()

	var bad UserFlag = 64
	var f Flags[UserFlag]
	_ = f.Set(bad)
}

func TestFromInt64NormalizeSignBit(t *testing.T) {
	f := FromInt64[UserFlag](-1)

	if f.Int64() != math.MaxInt64 {
		t.Fatalf("expected normalized value=%d, got %d", int64(math.MaxInt64), f.Int64())
	}
	if f.Len() != MaxBit {
		t.Fatalf("expected len=%d, got %d", MaxBit, f.Len())
	}
	if got := len(f.Enums()); got != MaxBit {
		t.Fatalf("expected enums len=%d, got %d", MaxBit, got)
	}
}

func TestScanNormalizeSignBit(t *testing.T) {
	var f Flags[UserFlag]
	if err := f.Scan(int64(-1)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if f.Int64() != math.MaxInt64 {
		t.Fatalf("expected normalized value=%d, got %d", int64(math.MaxInt64), f.Int64())
	}
	if !f.Has(UserFlagVerified) || !f.Has(UserFlagFeatureX) {
		t.Fatal("expected low bits to remain set after normalization")
	}
}

func TestContainsIntersectsIgnoreSignBit(t *testing.T) {
	f := Flags[UserFlag](-1)
	must := MaskOf[UserFlag](UserFlagVerified, UserFlagVIP)
	none := Flags[UserFlag](0)

	if !f.Contains(must) {
		t.Fatal("expected Contains=true for normalized low bits")
	}
	if !f.Intersects(must) {
		t.Fatal("expected Intersects=true for normalized low bits")
	}
	if f.Intersects(none) {
		t.Fatal("expected Intersects=false for zero mask")
	}
}
