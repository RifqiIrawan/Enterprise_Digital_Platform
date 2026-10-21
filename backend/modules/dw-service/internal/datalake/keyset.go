package datalake

import "encoding/hex"

// KeySet adalah himpunan kunci Silver ("<CompanyID>/<id>") untuk daftar kunci
// hidup. Kunci yang kedua bagiannya UUID kanonik (huruf kecil, bertanda hubung)
// disimpan sebagai 32 byte tetap tanpa pointer, bukan string ~73 byte plus
// header: pada ratusan ribu baris itu memangkas memori peta beberapa kali lipat.
// Kunci selain itu jatuh ke peta string, jadi semantiknya TETAP persis sama
// dengan perbandingan string: dua ejaan UUID yang berbeda tetap dua kunci
// berbeda, sama seperti di Silver.
type KeySet struct {
	compact map[[32]byte]struct{}
	other   map[string]struct{}
}

func NewKeySet(sizeHint int) *KeySet {
	return &KeySet{compact: make(map[[32]byte]struct{}, sizeHint)}
}

func (s *KeySet) Add(key string) {
	if k, ok := compactKey(key); ok {
		s.compact[k] = struct{}{}
		return
	}
	if s.other == nil {
		s.other = map[string]struct{}{}
	}
	s.other[key] = struct{}{}
}

func (s *KeySet) Has(key string) bool {
	if k, ok := compactKey(key); ok {
		_, found := s.compact[k]
		return found
	}
	_, found := s.other[key]
	return found
}

func (s *KeySet) Len() int { return len(s.compact) + len(s.other) }

// compactKey memadatkan "uuid/uuid" ke 32 byte. Hanya bentuk kanonik yang
// dipadatkan, supaya padat -> string -> padat tidak pernah menyatukan dua
// kunci yang berbeda sebagai string.
func compactKey(key string) ([32]byte, bool) {
	var out [32]byte
	if len(key) != 73 || key[36] != '/' {
		return out, false
	}
	if !uuidBytes(key[:36], out[:16]) || !uuidBytes(key[37:], out[16:]) {
		return out, false
	}
	return out, true
}

func uuidBytes(s string, dst []byte) bool {
	var hexBuf [32]byte
	n := 0
	for i := 0; i < 36; i++ {
		c := s[i]
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
			continue
		}
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
		hexBuf[n] = c
		n++
	}
	_, err := hex.Decode(dst, hexBuf[:])
	return err == nil
}
