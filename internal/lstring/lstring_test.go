package lstring

import (
	"testing"
)

/*
** Test MEMERRMSG constant
 */
func TestMemErrMsg(t *testing.T) {
	if MEMERRMSG != "not enough memory" {
		t.Errorf("MEMERRMSG should be 'not enough memory', got '%s'", MEMERRMSG)
	}
}

/*
** Test string table size constants
 */
func TestStringConstants(t *testing.T) {
	if MINSTRTABSIZE != 128 {
		t.Errorf("MINSTRTABSIZE should be 128, got %d", MINSTRTABSIZE)
	}
	if MAXSTRTB != 1<<31-1 {
		t.Errorf("MAXSTRTB should be 2^31-1, got %d", MAXSTRTB)
	}
}

/*
** Test HashString function
 */
func TestHashString(t *testing.T) {
	t.Run("Hash is deterministic", func(t *testing.T) {
		str := []byte("hello world")
		h1 := HashString(str, 0)
		h2 := HashString(str, 0)
		if h1 != h2 {
			t.Error("HashString should be deterministic")
		}
	})

	t.Run("Different seeds produce different hashes", func(t *testing.T) {
		str := []byte("hello world")
		h1 := HashString(str, 0)
		h2 := HashString(str, 1)
		if h1 == h2 {
			t.Error("Different seeds should produce different hashes")
		}
	})

	t.Run("Different strings produce different hashes", func(t *testing.T) {
		str1 := []byte("hello")
		str2 := []byte("world")
		h1 := HashString(str1, 0)
		h2 := HashString(str2, 0)
		if h1 == h2 {
			t.Error("Different strings should produce different hashes")
		}
	})

	t.Run("Empty string hash", func(t *testing.T) {
		str := []byte("")
		h := HashString(str, 0)
		if h != 0 {
			t.Errorf("Hash of empty string with seed 0 should be 0, got %d", h)
		}
	})

	t.Run("Hash includes length", func(t *testing.T) {
		str1 := []byte("a")
		str2 := []byte("aa")
		h1 := HashString(str1, 0)
		h2 := HashString(str2, 0)
		if h1 == h2 {
			t.Error("Strings of different lengths should produce different hashes")
		}
	})
}

/*
** Test function existence by using them
 */
func TestFunctionsExist(t *testing.T) {
	// Just verify the functions are accessible (use them to avoid compiler warning)
	_ = IsReserved
	_ = EqStr
	_ = StrLen
	_ = GetStr
	_ = HashLongStr
	_ = getShrStr
	_ = getLngStr
}