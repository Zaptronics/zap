//go:build darwin

// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const secureEnclaveHelper = `
import Foundation
import Security
import LocalAuthentication
func fail(_ message: String) -> Never {
 FileHandle.standardError.write(Data((message + "\n").utf8)); exit(1)
}
let args = CommandLine.arguments
if args.count != 3 { fail("invalid Secure Enclave request") }
let action = args[1], id = args[2]
let tag = Data(id.utf8)
var error: Unmanaged<CFError>?
var key: SecKey
if action == "create" {
 guard let access = SecAccessControlCreateWithFlags(nil, kSecAttrAccessibleWhenUnlockedThisDeviceOnly, [.privateKeyUsage, .userPresence], &error) else { fail("Cannot create user-presence policy") }
 let attributes: [String: Any] = [
  kSecAttrKeyType as String: kSecAttrKeyTypeECSECPrimeRandom,
  kSecAttrKeySizeInBits as String: 256,
  kSecAttrTokenID as String: kSecAttrTokenIDSecureEnclave,
  kSecPrivateKeyAttrs as String: [
   kSecAttrIsPermanent as String: true,
   kSecAttrApplicationTag as String: tag,
   kSecAttrAccessControl as String: access
  ]
 ]
 guard let created = SecKeyCreateRandomKey(attributes as CFDictionary, &error) else { fail("Secure Enclave key creation unavailable or declined: \(String(describing: error?.takeRetainedValue()))") }
 key = created
} else if action == "sign" {
 let context = LAContext()
 context.localizedReason = "Approve signing of the reviewed Zap source baseline"
 context.touchIDAuthenticationAllowableReuseDuration = 0
 let query: [String: Any] = [
  kSecClass as String: kSecClassKey,
  kSecAttrApplicationTag as String: tag,
  kSecAttrKeyType as String: kSecAttrKeyTypeECSECPrimeRandom,
  kSecAttrTokenID as String: kSecAttrTokenIDSecureEnclave,
  kSecReturnRef as String: true,
  kSecUseAuthenticationContext as String: context
 ]
 var result: CFTypeRef?
 let status = SecItemCopyMatching(query as CFDictionary, &result)
 guard status == errSecSuccess, let found = result else { fail("Secure Enclave key unavailable or approval declined (\(status))") }
 key = (found as! SecKey)
} else { fail("invalid operation") }
guard let attributes = SecKeyCopyAttributes(key) as? [String: Any],
 let token = attributes[kSecAttrTokenID as String] as? String,
 token == (kSecAttrTokenIDSecureEnclave as String) else { fail("Key is not in the Secure Enclave") }
if action == "create" {
 guard let pub = SecKeyCopyPublicKey(key), let raw = SecKeyCopyExternalRepresentation(pub, &error) as Data? else { fail("Cannot export public key") }
 print(raw.base64EncodedString())
} else {
 let raw = FileHandle.standardInput.readDataToEndOfFile()
 guard raw.count == 32 else { fail("Invalid signing digest") }
 guard let sig = SecKeyCreateSignature(key, .ecdsaSignatureDigestX962SHA256, raw as CFData, &error) as Data? else { fail("Signing unavailable or declined: \(String(describing: error?.takeRetainedValue()))") }
 print(sig.base64EncodedString())
}
`

func enclaveCall(action, id string, digest []byte) ([]byte, error) {
	if _, err := os.Stat("/usr/bin/swift"); err != nil {
		return nil, fmt.Errorf("Secure Enclave bridge requires Apple Swift command-line tools; no software fallback")
	}
	dir, err := os.MkdirTemp("", "zap-enclave-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	source := filepath.Join(dir, "sign.swift")
	if err := os.WriteFile(source, []byte(secureEnclaveHelper), 0600); err != nil {
		return nil, err
	}
	cmd := exec.Command("/usr/bin/swift", source, action, id)
	cmd.Dir = dir
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if name == "DEVELOPER_DIR" || name == "SDKROOT" || name == "PATH" || strings.HasPrefix(name, "DYLD_") || strings.HasPrefix(name, "SWIFT_") {
			continue
		}
		cmd.Env = append(cmd.Env, entry)
	}
	cmd.Env = append(cmd.Env, "PATH=/usr/bin:/bin:/usr/sbin:/sbin")
	cmd.Stdin = bytes.NewReader(digest)
	cmd.Stderr = os.Stderr
	result, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("Secure Enclave operation failed: %w", err)
	}
	return base64.StdEncoding.DecodeString(strings.TrimSpace(string(result)))
}
func platformHardwareCreate(id string) (string, string, error) {
	raw, err := enclaveCall("create", id, nil)
	if err != nil {
		return "", "", err
	}
	x, y := elliptic.Unmarshal(elliptic.P256(), raw)
	if x == nil {
		return "", "", fmt.Errorf("invalid Secure Enclave public key")
	}
	public, err := hardwarePublic(&ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y})
	return "macos-enclave:" + id, public, err
}
func platformHardwareSign(reference string, digest []byte) ([]byte, error) {
	if !strings.HasPrefix(reference, "macos-enclave:ZapIntegrity-") {
		return nil, fmt.Errorf("expected Secure Enclave key reference")
	}
	return enclaveCall("sign", strings.TrimPrefix(reference, "macos-enclave:"), digest)
}
