// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"log"
	"time"

	"github.com/go-faster/xor"
	kmsservice "k8s.io/kms/pkg/service"
)

var (
	addr = flag.String("listen", "/var/run/kmsplugin/socket.sock", "path to bind the unix socket to")
	key  = flag.String("key", "", "key to use for encryption")
)

func main() {
	flag.Parse()
	if *key == "" {
		log.Fatal("key must not be empty")
	}
	grpcSvc := kmsservice.NewGRPCService(*addr, time.Second*5, svc{
		key: *key,
	})
	log.Printf("serving on %s", *addr)
	if err := grpcSvc.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

type svc struct {
	key string
}

// Decrypt implements service.Service.
func (s svc) Decrypt(_ context.Context, _ string, req *kmsservice.DecryptRequest) ([]byte, error) {
	if req.KeyID != s.key {
		return nil, nil
	}
	return decrypt([]byte(s.key), req.Ciphertext), nil
}

// Encrypt implements service.Service.
func (s svc) Encrypt(_ context.Context, _ string, data []byte) (*kmsservice.EncryptResponse, error) {
	ciphertext := encrypt([]byte(s.key), data)
	return &kmsservice.EncryptResponse{
		Ciphertext: ciphertext,
		KeyID:      s.key,
	}, nil
}

// Status implements service.Service.
func (s svc) Status(_ context.Context) (*kmsservice.StatusResponse, error) {
	return &kmsservice.StatusResponse{
		Version: "v2",
		Healthz: "ok",
		KeyID:   s.key,
	}, nil
}

func encrypt(key, plain []byte) []byte {
	cipher := make([]byte, len(plain))
	xor.Bytes(cipher, plain, key)
	return cipher
}

func decrypt(key, cipher []byte) []byte {
	plain := make([]byte, len(cipher))
	xor.Bytes(plain, cipher, key)
	return plain
}
