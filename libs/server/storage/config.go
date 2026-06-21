package storage

// DriverName returns the storage driver name from env, defaulting to "fs".
func DriverName(v string) string {
	if v == "" {
		return "fs"
	}
	return v
}

// DriverFS is the constant for the filesystem driver.
const DriverFS = "fs"

// Credentials holds access keys for S3-compatible storage.
type Credentials struct {
	AccessKey string
	SecretKey string
}

// Config is the full configuration for an object-store backend.
type Config struct {
	Driver      string
	FSRoot      string
	Endpoint    string
	Region      string
	Bucket      string
	BasePath    string
	Credentials Credentials
}

const (
	DriverMinIO = "minio"
	DriverS3    = "s3"
	DriverR2    = "r2"
)

