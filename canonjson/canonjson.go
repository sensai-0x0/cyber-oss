// Package canonjson writes the canonical JSON form that orchestrator/ledger.py hashes and
// that mitigation action v2 signs (ADR 0005): sorted keys, no whitespace, UTF-8, Python's
// string escapes and float formatting. Shared golden vectors pin it against Python.
package canonjson

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Member is one key of an Object.
type Member struct {
	Key   string
	Value any
}

// Object keeps the given key order, for signed forms whose field order is fixed.
type Object []Member

// Canonicalize re-encodes one JSON document in canonical form. It rejects anything
// Python's writer cannot have produced: trailing data, duplicate keys, invalid UTF-8
// and non-finite numbers.
func Canonicalize(raw []byte) ([]byte, error) {
	if !utf8.Valid(raw) {
		return nil, errors.New("canonjson: invalid UTF-8")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	value, err := parse(decoder)
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, errors.New("canonjson: trailing data after the document")
	}
	var out bytes.Buffer
	if err := Write(&out, value); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func parse(decoder *json.Decoder) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("canonjson: %w", err)
	}
	delim, isDelim := token.(json.Delim)
	if !isDelim {
		return token, nil
	}
	if delim == '[' {
		items := []any{}
		for decoder.More() {
			item, err := parse(decoder)
			if err != nil {
				return nil, err
			}
			items = append(items, item)
		}
		_, err := decoder.Token()
		return items, err
	}
	members := map[string]any{}
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("canonjson: %w", err)
		}
		key, _ := keyToken.(string)
		if _, dup := members[key]; dup {
			return nil, errors.New("canonjson: duplicate object key")
		}
		if members[key], err = parse(decoder); err != nil {
			return nil, err
		}
	}
	_, err = decoder.Token()
	return members, err
}

// Write encodes value canonically. Supported: nil, bool, string, json.Number, int, int64,
// float64, []any, map[string]any (keys sorted) and Object (given order).
func Write(w io.Writer, value any) error {
	var out strings.Builder
	if err := encode(&out, value); err != nil {
		return err
	}
	_, err := io.WriteString(w, out.String())
	return err
}

func encode(out *strings.Builder, value any) error {
	switch v := value.(type) {
	case nil:
		out.WriteString("null")
	case bool:
		out.WriteString(strconv.FormatBool(v))
	case string:
		return writeString(out, v)
	case int:
		out.WriteString(strconv.Itoa(v))
	case int64:
		out.WriteString(strconv.FormatInt(v, 10))
	case float64:
		text, err := formatFloat(v)
		if err != nil {
			return err
		}
		out.WriteString(text)
	case json.Number:
		return writeNumber(out, v)
	case []any:
		out.WriteByte('[')
		for i, item := range v {
			if i > 0 {
				out.WriteByte(',')
			}
			if err := encode(out, item); err != nil {
				return err
			}
		}
		out.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys) // UTF-8 byte order equals code point order
		members := make(Object, len(keys))
		for i, key := range keys {
			members[i] = Member{key, v[key]}
		}
		return encode(out, members)
	case Object:
		out.WriteByte('{')
		for i, member := range v {
			if i > 0 {
				out.WriteByte(',')
			}
			if err := writeString(out, member.Key); err != nil {
				return err
			}
			out.WriteByte(':')
			if err := encode(out, member.Value); err != nil {
				return err
			}
		}
		out.WriteByte('}')
	default:
		return fmt.Errorf("canonjson: unsupported type %T", value)
	}
	return nil
}

// writeString follows Python's json.dumps(ensure_ascii=False): only quote, backslash and
// control characters are escaped. HTML characters and U+2028 stay raw.
func writeString(out *strings.Builder, text string) error {
	if !utf8.ValidString(text) {
		return errors.New("canonjson: invalid UTF-8 in string")
	}
	out.WriteByte('"')
	for _, r := range text {
		switch r {
		case '"':
			out.WriteString(`\"`)
		case '\\':
			out.WriteString(`\\`)
		case '\n':
			out.WriteString(`\n`)
		case '\r':
			out.WriteString(`\r`)
		case '\t':
			out.WriteString(`\t`)
		case '\b':
			out.WriteString(`\b`)
		case '\f':
			out.WriteString(`\f`)
		default:
			if r < 0x20 {
				fmt.Fprintf(out, `\u%04x`, r)
			} else {
				out.WriteRune(r)
			}
		}
	}
	out.WriteByte('"')
	return nil
}

func writeNumber(out *strings.Builder, number json.Number) error {
	text := number.String()
	if !strings.ContainsAny(text, ".eE") { // an integer keeps its digits, however large
		if text == "-0" {
			text = "0"
		}
		out.WriteString(text)
		return nil
	}
	f, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return fmt.Errorf("canonjson: bad number %q", text)
	}
	formatted, err := formatFloat(f)
	if err != nil {
		return err
	}
	out.WriteString(formatted)
	return nil
}

// formatFloat reproduces Python's repr(float): shortest round-trip digits, positional
// notation for decimal exponents -4 through 15, otherwise d.ddde±XX with two exponent digits.
func formatFloat(f float64) (string, error) {
	if math.IsInf(f, 0) || math.IsNaN(f) {
		return "", errors.New("canonjson: non-finite number")
	}
	sign := ""
	if math.Signbit(f) {
		sign, f = "-", -f
	}
	if f == 0 {
		return sign + "0.0", nil
	}
	sci := strconv.FormatFloat(f, 'e', -1, 64) // d[.ddd]e±XX
	mantissa, expText, _ := strings.Cut(sci, "e")
	exp, _ := strconv.Atoi(expText)
	digits := strings.Replace(mantissa, ".", "", 1)
	if exp < -4 || exp > 15 {
		if len(digits) > 1 {
			mantissa = digits[:1] + "." + digits[1:]
		} else {
			mantissa = digits
		}
		esign := "+"
		if exp < 0 {
			esign, exp = "-", -exp
		}
		return fmt.Sprintf("%s%se%s%02d", sign, mantissa, esign, exp), nil
	}
	switch {
	case exp < 0:
		return sign + "0." + strings.Repeat("0", -exp-1) + digits, nil
	case len(digits) <= exp+1:
		return sign + digits + strings.Repeat("0", exp+1-len(digits)) + ".0", nil
	default:
		return sign + digits[:exp+1] + "." + digits[exp+1:], nil
	}
}
