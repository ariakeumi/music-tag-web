package scraper

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/big"
	"math/rand"
	"strings"
)

// NetEase weapi/linuxapi encryption, ported from applications/utils/encrypt.py.

const (
	neteaseModulus = "00e0b509f6259df8642dbc35662901477df22677ec152b5ff68ace615bb7" +
		"b725152b3ab17a876aea8a5aa76d2e417629ec4ee341f56135fccf695280" +
		"104e0312ecbda92557c93870114af6c9d05c4f7f0c3685b7a46bee255932" +
		"575cce10b424d813cfe4875d3e82047b97ddef52741d546b8e289dc6935b" +
		"3ece0462db0a22b8e7"
	neteasePubKey = "010001"
	neteaseNonce  = "0CoJUm6Qyw8W8jud"
	linuxAESKey   = "rFgB&h#%2?^eDg:Q"
)

func md5Hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

// aesPadEncrypt applies the Python aes(): manual 16-byte padding, CBC with
// fixed IV when iv is set, else ECB; base64 or uppercase hex output.
func aesPadEncrypt(data []byte, key string, iv bool, b64 bool) (string, error) {
	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		return "", err
	}
	pad := 16 - len(data)%16
	padded := append(data, bytes.Repeat([]byte{byte(pad)}, pad)...)
	out := make([]byte, len(padded))
	if iv {
		cipher.NewCBCEncrypter(block, []byte("0102030405060708")).CryptBlocks(out, padded)
	} else {
		// stdlib has no ECB; it is just independent block encryptions
		for i := 0; i+block.BlockSize() <= len(padded); i += block.BlockSize() {
			block.Encrypt(out[i:i+block.BlockSize()], padded[i:i+block.BlockSize()])
		}
	}
	if b64 {
		return base64.StdEncoding.EncodeToString(out), nil
	}
	return strings.ToUpper(hex.EncodeToString(out)), nil
}

// rsaNoPad replicates the Python rsa(): the secret is reversed byte-wise,
// treated as a big integer and exponentiated raw (no padding).
func rsaNoPad(secret string) string {
	runes := []byte(secret)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	m := new(big.Int).SetBytes(runes)
	e := new(big.Int)
	e.SetString(neteasePubKey, 16)
	n := new(big.Int)
	n.SetString(neteaseModulus, 16)
	result := new(big.Int).Exp(m, e, n)
	return fmt.Sprintf("%0256x", result)
}

func randomHexKey(size int) string {
	buf := make([]byte, size)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)[:size]
}

func weEncrypt(text string) (params, encSecKey string, err error) {
	secret := randomHexKey(16)
	first, err := aesPadEncrypt([]byte(text), neteaseNonce, true, true)
	if err != nil {
		return "", "", err
	}
	params, err = aesPadEncrypt([]byte(first), secret, true, true)
	if err != nil {
		return "", "", err
	}
	return params, rsaNoPad(secret), nil
}

func linuxEncrypt(text string) (string, error) {
	return aesPadEncrypt([]byte(text), linuxAESKey, false, false)
}
