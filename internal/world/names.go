package world

import (
	"math/rand"
	"strings"
)

// Phonotactics for in-world place names. The cradle reads as
// Mediterranean / Anatolian in the lore (Halys, Maeander, Sangarios,
// Tigris, Euphrates flavor), so we lean toward open CV syllables and
// vowel-coda endings.
var (
	nameOnsets = []string{
		"k", "t", "h", "m", "s", "p", "n", "l", "r",
		"kh", "th", "tr", "kr", "br", "dr", "pr", "sk",
	}
	nameVowels = []string{"a", "e", "i", "o", "u"}
	nameCodas  = []string{
		"os", "is", "us", "as", "es",
		"an", "on", "ar", "or", "yr",
	}
)

// generateName builds a deterministic 2-3 syllable place name from a
// seed. Schema: (CV){nSyl-1} + C+coda. The final coda is V-C so the
// whole name ends in a consonant cluster like -os / -is / -an, which
// reads as Anatolian/Mediterranean.
//
// Seed should encode whatever invariant identifies the place — for
// rivers, (worldSeed, headX, headY); for seats, (worldSeed, x, y).
// Same seed → same name forever.
func generateName(rngSeed int64) string {
	rng := rand.New(rand.NewSource(rngSeed))
	nSyl := 2 + rng.Intn(2)
	var b strings.Builder
	for i := 0; i < nSyl-1; i++ {
		b.WriteString(nameOnsets[rng.Intn(len(nameOnsets))])
		b.WriteString(nameVowels[rng.Intn(len(nameVowels))])
	}
	b.WriteString(nameOnsets[rng.Intn(len(nameOnsets))])
	b.WriteString(nameCodas[rng.Intn(len(nameCodas))])
	name := b.String()
	if name == "" {
		return ""
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

// nameSeedForCell mixes world seed with a cell coordinate to produce a
// stable per-cell name seed. Multiplication by primes spreads the
// distribution so adjacent cells get unrelated names. Pure function;
// no global state.
func nameSeedForCell(worldSeed int64, x, y int64) int64 {
	return worldSeed*1000003 + x*7919 + y*31
}
