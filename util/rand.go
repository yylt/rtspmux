package util

import "math/rand"

var (
	letters = []rune("0123456789abcdefghijklmnopqrstuvwxyz")
)

func RandStr(length int) string {
	b := make([]rune, length)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}
