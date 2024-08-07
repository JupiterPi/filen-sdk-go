package crypto

import (
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"golang.org/x/crypto/pbkdf2"
)

func DeriveKeyFromPassword(password string, salt string, iterations int, bitLength int) []byte {
	return pbkdf2.Key([]byte(password), []byte(salt), iterations, bitLength/8, sha512.New)
}

func GeneratePasswordAndMasterKey(rawPassword string, salt string) (derivedMasterKey string, derivedPassword string) {
	derivedKey := hex.EncodeToString(DeriveKeyFromPassword(rawPassword, salt, 200000, 512))
	derivedMasterKey, derivedPassword = derivedKey[:len(derivedKey)/2], derivedKey[len(derivedKey)/2:]
	derivedPassword = fmt.Sprintf("%032x", runSHA521(derivedPassword))
	return
}
