// Command storage-setup creates the media bucket if it doesn't exist and
// lets browsers upload to it from CAVI_ALLOWED_ORIGINS. Run it once per
// environment (make storage-setup locally). Its credentials need bucket
// administration rights; the API itself only needs object read/write.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/lupppig/cavi/apps/api/internal/config"
	"github.com/lupppig/cavi/apps/api/internal/storage"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "storage-setup:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	if !cfg.StorageEnabled() {
		return fmt.Errorf("CAVI_STORAGE_ENDPOINT is not set")
	}
	if len(cfg.AllowedOrigins) == 0 {
		return fmt.Errorf("CAVI_ALLOWED_ORIGINS is empty; browsers would not be able to upload")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	st := cfg.Storage
	s := storage.NewS3(storage.S3Config{
		Endpoint: st.Endpoint, PublicEndpoint: st.PublicEndpoint, Region: st.Region, Bucket: st.Bucket,
		AccessKeyID: st.AccessKeyID, SecretAccessKey: st.SecretAccessKey, PathStyle: st.PathStyle,
	})
	if err := s.Setup(ctx, cfg.AllowedOrigins); err != nil {
		return err
	}
	fmt.Printf("bucket %q ready; browser uploads allowed from %v\n", st.Bucket, cfg.AllowedOrigins)
	return nil
}
