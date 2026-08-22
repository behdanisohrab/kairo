package sni

import (
	"crypto/md5"
	"crypto/tls"
	"fmt"
	"strings"
)

// ComputeJA3 computes the JA3 fingerprint from a TLS ClientHello.
// JA3 is a method for creating SSL/TLS client fingerprints that are
// easy to produce and share. See https://github.com/salesforce/ja3.
func ComputeJA3(hello *tls.ClientHelloInfo) string {
	ja3 := buildJA3String(hello)
	hash := md5.Sum([]byte(ja3))
	return fmt.Sprintf("%x", hash)
}

// ComputeJA3Raw returns the raw JA3 string (before hashing) for debugging.
func ComputeJA3Raw(hello *tls.ClientHelloInfo) string {
	return buildJA3String(hello)
}

func buildJA3String(hello *tls.ClientHelloInfo) string {
	var b strings.Builder

	// TLS version
	b.WriteString(fmt.Sprintf("%d", hello.SupportedVersions))
	b.WriteString(",")

	// Cipher suites
	ciphers := make([]string, 0, len(hello.CipherSuites))
	for _, c := range hello.CipherSuites {
		ciphers = append(ciphers, fmt.Sprintf("%d", c))
	}
	b.WriteString(strings.Join(ciphers, "-"))
	b.WriteString(",")

	// Extensions
	exts := make([]string, 0, len(hello.Extensions))
	for _, e := range hello.Extensions {
		exts = append(exts, fmt.Sprintf("%d", e))
	}
	b.WriteString(strings.Join(exts, "-"))
	b.WriteString(",")

	// Elliptic curves (SupportedGroups)
	if hello.SupportedCurves != nil {
		curves := make([]string, 0, len(hello.SupportedCurves))
		for _, c := range hello.SupportedCurves {
			curves = append(curves, fmt.Sprintf("%d", c))
		}
		b.WriteString(strings.Join(curves, "-"))
	}
	b.WriteString(",")

	// Elliptic curve point formats
	if hello.SupportedPoints != nil {
		points := make([]string, 0, len(hello.SupportedPoints))
		for _, p := range hello.SupportedPoints {
			points = append(points, fmt.Sprintf("%d", p))
		}
		b.WriteString(strings.Join(points, "-"))
	}

	return b.String()
}
