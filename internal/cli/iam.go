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

package cli

import "github.com/sacloud/skr/internal/iamapi"

// initIAMUser2FA は User2FA コマンドの配線を行います。factory_arguments (user-id) を factory に渡すため手書きです (ADR 0068)。
// その他の IAM コマンドの配線は iam_wiring_generated.go が行います。
func (c *cli) initIAMUser2FA() {
	c.IAMAPI.User2FA = iamapi.NewUser2FACommands(iamapi.User2FARuntime{ValidateOutput: validateOutput, WriteOutput: writeOutput}, func(userID int) (iamapi.User2FAAPI, error) {
		return iamapi.NewUser2FAAPI(c.Trace, userID)
	})
}
