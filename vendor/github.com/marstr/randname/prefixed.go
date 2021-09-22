package randname

import (
	"bytes"
	"crypto/rand"
	"io"
	"math/big"
)

// SpecialCharacters holds a host of standard special characters.
var SpecialCharacters = []rune{'^', '$', '#', '*', '(', ')', '%', '@'}

// LowercaseAlphabet contains each letter of the alphaber, in lowercase.
var LowercaseAlphabet = []rune{'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j', 'k', 'l', 'm', 'n', 'o', 'p', 'q', 'r', 's', 't', 'u', 'v', 'w', 'x', 'y', 'z'}

// UppercaseAlphabet contains each letter of the alphabet, in uppercase.
var UppercaseAlphabet = []rune{'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'I', 'J', 'K', 'L', 'M', 'N', 'O', 'P', 'Q', 'R', 'S', 'T', 'U', 'V', 'W', 'X', 'Y', 'Z'}

// ArabicNumerals contains the numeric digits 0-9.
var ArabicNumerals = []rune{'0', '1', '2', '3', '4', '5', '6', '7', '8', '9'}

// PrefixedDefaultAcceptable contains the characters that will be tacked onto the end of the Prefix, if none were provided.
var PrefixedDefaultAcceptable = append(append(append([]rune{}, ArabicNumerals...), UppercaseAlphabet...), LowercaseAlphabet...)

// PrefixedDefaultLen is the number of characters that will be appended to a suffix if no length was specified.
const PrefixedDefaultLen = uint8(6)

// Prefixed offers a means of generating a random name by providing a prefix, followed by a number of random letters or numbers.
type Prefixed struct {
	Prefix        string
	Acceptable    []rune
	Len           uint8
	RandGenerator io.Reader
}

// Generate creates a string starting with a prefix, and ending with an assortment of randomly chosen runes.
func (p Prefixed) Generate() string {
	if p.Len == 0 {
		p.Len = PrefixedDefaultLen
	}

	if len(p.Acceptable) == 0 {
		p.Acceptable = PrefixedDefaultAcceptable
	}

	if p.RandGenerator == nil {
		p.RandGenerator = rand.Reader
	}

	builder := bytes.NewBufferString(p.Prefix)

	max := big.NewInt(int64(len(p.Acceptable)))

	for i := uint8(0); i < p.Len; i++ {
		result, err := rand.Int(p.RandGenerator, max)
		if err != nil {
			panic(err)
		}
		builder.WriteRune(p.Acceptable[result.Int64()])
	}
	return builder.String()
}

// GenerateWithPrefix generates a string with a given prefix then a given number of randomly selected characters.
func GenerateWithPrefix(prefix string, characters uint8) string {
	return Prefixed{Prefix: prefix, Len: characters}.Generate()
}
