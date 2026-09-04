package javdb

import (
	"crypto/md5"
	"encoding/hex"
	"strconv"
)

// APK 1.9.28 derives these values from access key 30820 and its embedded
// signature constants. Only the Unix timestamp changes between requests.
const (
	signaturePrefix = "71cf27bb3c0bcdf207b64abecddc970098c7421ee7203b9cdae54478478a199e7d5a6e1a57691123c1a931c057842fb73ba3b3c83bcd69c17ccf174081e3d8aa"
	signatureSuffix = "lpw6vgqzsp"
)

func signature(timestamp int64) string {
	seconds := strconv.FormatInt(timestamp, 10)
	sum := md5.Sum([]byte(seconds + signaturePrefix))
	return seconds + "." + signatureSuffix + "." + hex.EncodeToString(sum[:])
}
