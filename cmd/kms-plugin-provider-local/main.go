// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"flag"
	"fmt"
	"io"
	"log"
	"time"

	kmsservice "k8s.io/kms/pkg/service"

	"github.com/gardener/gardener/pkg/utils"
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

	sha256Key := utils.SHA256([]byte(*key))

	grpcSvc := kmsservice.NewGRPCService(*addr, time.Second*5, svc{
		keyID: *key,
		key:   sha256Key,
	})
	log.Printf("serving on %s", *addr)
	if err := grpcSvc.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

type svc struct {
	keyID string
	key   []byte
}

// Decrypt implements service.Service.
func (s svc) Decrypt(_ context.Context, uid string, req *kmsservice.DecryptRequest) ([]byte, error) {
	log.Printf("Got decrypt request with uid %s and keyid %s, expected keyid %s", uid, req.KeyID, s.keyID)
	if req.KeyID != s.keyID {
		return nil, nil
	}
	return decrypt(s.key, req.Ciphertext)
}

// Encrypt implements service.Service.
func (s svc) Encrypt(_ context.Context, uid string, data []byte) (*kmsservice.EncryptResponse, error) {
	log.Printf("Returning encrypt request with keyid %s and uid %s", s.keyID, uid)
	ciphertext, err := encrypt(s.key, data)
	if err != nil {
		return nil, err
	}

	return &kmsservice.EncryptResponse{
		Ciphertext: ciphertext,
		KeyID:      s.keyID,
	}, nil
}

// Status implements service.Service.
func (s svc) Status(_ context.Context) (*kmsservice.StatusResponse, error) {
	return &kmsservice.StatusResponse{
		Version: "v2",
		Healthz: "ok",
		KeyID:   s.keyID,
	}, nil
}

func encrypt(key, plain []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	ciphertext := make([]byte, aes.BlockSize+len(plain))
	iv := ciphertext[:aes.BlockSize]
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, err
	}

	stream := cipher.NewCFBEncrypter(block, iv)
	stream.XORKeyStream(ciphertext[aes.BlockSize:], plain)

	return ciphertext, nil
}

func decrypt(key, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	if len(ciphertext) < aes.BlockSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	iv := ciphertext[:aes.BlockSize]
	ciphertext = ciphertext[aes.BlockSize:]

	stream := cipher.NewCFBDecrypter(block, iv)
	stream.XORKeyStream(ciphertext, ciphertext)

	return ciphertext, nil
}
