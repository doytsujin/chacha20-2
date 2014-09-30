// Package chacha20 provides a pure Go implementation of ChaCha20, a fast, secure
// stream cipher.
//
// From Bernstein, Daniel J. "ChaCha, a variant of Salsa20." Workshop Record of
// SASC. 2008. (http://cr.yp.to/chacha/chacha-20080128.pdf):
//
//	ChaCha8 is a 256-bit stream cipher based on the 8-round cipher Salsa20/8.
//	The changes from Salsa20/8 to ChaCha8 are designed to improve diffusion per
//	round, conjecturally increasing resistance to cryptanalysis, while
//	preserving—and often improving—time per round. ChaCha12 and ChaCha20 are
//	analogous modiﬁcations of the 12-round and 20-round ciphers Salsa20/12 and
//	Salsa20/20. This paper presents the ChaCha family and explains the
//	differences between Salsa20 and ChaCha.
//
// For more information, see http://cr.yp.to/chacha.html
package chacha20

import (
	"crypto/cipher"
	"encoding/binary"
	"errors"
	"unsafe"
)

const (
	KeySize   = 32 // KeySize is the length of ChaCha20 keys, in bytes.
	NonceSize = 8  // NonceSize is the length of ChaCha20 nonces, in bytes.

	stateSize = 16            // the size of ChaCha20's state, in words
	blockSize = stateSize * 4 // the size of ChaCha20's block, in bytes
)

var (
	// ErrInvalidKey is returned when the provided key is not 256 bits long.
	ErrInvalidKey = errors.New("chacha20: Invalid key length (must be 256 bits)")
	// ErrInvalidNonce is returned when the provided nonce is not 64 bits long.
	ErrInvalidNonce = errors.New("chacha20: Invalid nonce length (must be 64 bits)")

	bigEndian bool // we're running on a bigEndian CPU
)

// Do some up-front bookkeeping on what sort of CPU we're using. ChaCha20 treats
// its state as a little-endian byte array when it comes to generating the
// keystream, which allows for a zero-copy approach to the core transform. On
// big-endian architectures, we have to take a hit to reverse the bytes.
func init() {
	x := uint32(0x04030201)
	y := [4]byte{0x1, 0x2, 0x3, 0x4}
	bigEndian = *(*[4]byte)(unsafe.Pointer(&x)) != y
}

// A Cipher is an instance of ChaCha20 using a particular key and nonce.
type Cipher struct {
	state  [stateSize]uint32 // the state as an array of 16 32-bit words
	block  [blockSize]byte   // the keystream as an array of 64 bytes
	offset int               // the offset of used bytes in block
}

// NewCipher creates and returns a new Cipher.  The key argument must be 256
// bits long, and the nonce argument must be 64 bits long. The nonce must be
// randomly generated or used only once. This Cipher instance must not be used
// to encrypt more than 2^70 bytes (~1 zettabyte).
func NewCipher(key []byte, nonce []byte) (cipher.Stream, error) {
	if len(key) != KeySize {
		return nil, ErrInvalidKey
	}

	if len(nonce) != NonceSize {
		return nil, ErrInvalidNonce
	}

	s := new(stream)

	// the magic constants for 256-bit keys
	s.state[0] = 0x61707865
	s.state[1] = 0x3320646e
	s.state[2] = 0x79622d32
	s.state[3] = 0x6b206574

	s.state[4] = binary.LittleEndian.Uint32(key[0:])
	s.state[5] = binary.LittleEndian.Uint32(key[4:])
	s.state[6] = binary.LittleEndian.Uint32(key[8:])
	s.state[7] = binary.LittleEndian.Uint32(key[12:])
	s.state[8] = binary.LittleEndian.Uint32(key[16:])
	s.state[9] = binary.LittleEndian.Uint32(key[20:])
	s.state[10] = binary.LittleEndian.Uint32(key[24:])
	s.state[11] = binary.LittleEndian.Uint32(key[28:])

	s.state[12] = 0
	s.state[13] = 0
	s.state[14] = binary.LittleEndian.Uint32(nonce[0:])
	s.state[15] = binary.LittleEndian.Uint32(nonce[4:])

	s.advance()

	return s, nil
}

type stream struct {
	state  [stateSize]uint32 // the state as an array of 16 32-bit words
	block  [blockSize]byte   // the keystream as an array of 64 bytes
	offset int               // the offset of used bytes in block
}

func (s *stream) XORKeyStream(dst, src []byte) {
	// Stride over the input in 64-byte blocks, minus the amount of keystream
	// previously used. This will produce best results when processing blocks
	// of a size evenly divisible by 64.
	i := 0
	max := len(src)
	for i < max {
		gap := blockSize - s.offset

		limit := i + gap
		if limit > max {
			limit = max
		}

		o := s.offset
		for j := i; j < limit; j++ {
			dst[j] = src[j] ^ s.block[o]
			o++
		}

		i += gap
		s.offset = o

		if o == blockSize {
			s.advance()
		}
	}
}

// BUG(codahale): Totally untested on big-endian CPUs. Would very much
// appreciate someone with an ARM device giving this a swing.

// advances the keystream
func (s *stream) advance() {
	core(&s.state, (*[stateSize]uint32)(unsafe.Pointer(&s.block)))

	if bigEndian {
		j := blockSize - 1
		for i := 0; i < blockSize/2; i++ {
			s.block[j], s.block[i] = s.block[i], s.block[j]
			j--
		}
	}

	s.offset = 0
	i := s.state[12] + 1
	s.state[12] = i
	if i == 0 {
		s.state[13]++
	}
}
