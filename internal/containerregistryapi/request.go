// Copyright 2022-2026 The sacloud/skr Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package containerregistryapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

func DecodeRequest(input string, destination any) error {
	data, err := requestData(input)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("decode request JSON: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("decode request JSON: multiple JSON values")
		}
		return fmt.Errorf("decode request JSON: %w", err)
	}
	return nil
}

func requestData(input string) ([]byte, error) {
	if strings.HasPrefix(input, "@") {
		data, err := os.ReadFile(strings.TrimPrefix(input, "@"))
		if err != nil {
			return nil, fmt.Errorf("read request file: %w", err)
		}
		return data, nil
	}
	return []byte(input), nil
}

func ReadPasswordFile(path string) (string, error) {
	if path == "-" {
		return readPassword(os.Stdin, "read password from standard input")
	}
	file, err := os.Open(path) // #nosec G304 -- The password file path is explicitly provided by the CLI user.
	if err != nil {
		return "", fmt.Errorf("read password file: %w", err)
	}
	password, readErr := readPassword(file, "read password file")
	closeErr := file.Close()
	if readErr != nil || closeErr != nil {
		return "", errors.Join(readErr, wrapCloseError(closeErr))
	}
	return password, nil
}

func readPassword(reader io.Reader, operation string) (string, error) {
	data, err := io.ReadAll(io.LimitReader(reader, 66))
	if err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	password := strings.TrimSuffix(string(data), "\n")
	password = strings.TrimSuffix(password, "\r")
	return password, nil
}

func wrapCloseError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("close password file: %w", err)
}
