// Copyright 2023 StreamNative, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package notifications

import (
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"oxia/cmd/client/common"
)

var Cmd = &cobra.Command{
	Use:   "notifications",
	Short: "Get notifications stream",
	Long:  `Follow the change notifications stream`,
	Args:  cobra.NoArgs,
	RunE:  exec,
}

func exec(cmd *cobra.Command, args []string) error {
	client, err := common.Config.NewClient()
	if err != nil {
		return err
	}

	defer client.Close()

	notifications, err := client.GetNotifications()
	if err != nil {
		return err
	}

	defer notifications.Close()

	for notification := range notifications.Ch() {
		log.Info().
			Stringer("type", notification.Type).
			Str("key", notification.Key).
			Int64("version-id", notification.VersionId).
			Msg("")
	}

	return nil
}
