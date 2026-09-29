package multipasscli

import (
	"fmt"
	"math/big"
	"regexp"
	"strings"
)

var sizePattern = regexp.MustCompile(`(?i)^([0-9]+(?:\.[0-9]+)?)(B|[KMGT](?:I?B)?)?$`)

// ParseSize uses Multipass's binary units, flooring fractional bytes for unit
// suffixed values. Exact integer arithmetic avoids rounding large allocations.
func ParseSize(value string) (uint64, error) {
	matches := sizePattern.FindStringSubmatch(value)
	if matches == nil {
		return 0, fmt.Errorf("invalid size %q: use a positive byte count or a size such as 512M or 4G", value)
	}
	unit := strings.ToUpper(matches[2])
	if (unit == "" || unit == "B") && strings.Contains(matches[1], ".") {
		return 0, fmt.Errorf("size %q: fractional bytes are not supported", value)
	}
	// Parse the digits explicitly in base 10: big.Rat.SetString interprets
	// leading-zero integers as octal, while Multipass treats them as decimal.
	digits := strings.SplitN(matches[1], ".", 2)
	numerator, ok := new(big.Int).SetString(strings.Join(digits, ""), 10)
	if !ok {
		return 0, fmt.Errorf("invalid size %q", value)
	}
	denominator := big.NewInt(1)
	if len(digits) == 2 {
		denominator.Exp(big.NewInt(10), big.NewInt(int64(len(digits[1]))), nil)
	}
	number := new(big.Rat).SetFrac(numerator, denominator)
	multiplier := uint64(1)
	if unit != "" && unit != "B" {
		for i := 0; i <= strings.IndexByte("KMGT", unit[0]); i++ {
			multiplier *= 1024
		}
	}
	number.Mul(number, new(big.Rat).SetInt(new(big.Int).SetUint64(multiplier)))
	bytes := new(big.Int).Quo(number.Num(), number.Denom())
	if bytes.Sign() <= 0 || !bytes.IsInt64() {
		return 0, fmt.Errorf("size %q must be positive and no greater than 9223372036854775807 bytes", value)
	}
	return bytes.Uint64(), nil
}

// SizeMatchesReport recognizes Multipass's one-decimal, float32 size display.
// --raw currently only changes empty-value formatting, not size precision.
// Keep a previously requested allocation when this lossy report agrees with it;
// changes inside the same rounding interval cannot be detected by the CLI.
func SizeMatchesReport(known, reported uint64, display string) bool {
	if known == reported {
		return true
	}
	for _, unit := range []struct {
		bytes  uint64
		suffix string
	}{{1 << 30, "GiB"}, {1 << 20, "MiB"}, {1 << 10, "KiB"}} {
		quotient := float32(known) / float32(unit.bytes)
		if quotient >= 1 {
			return fmt.Sprintf("%.1f%s", float64(quotient), unit.suffix) == display
		}
	}
	return false
}
