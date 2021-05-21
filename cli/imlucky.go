/*
 * Copyright (c) 2018-present unTill Pro, Ltd. and Contributors
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 *
 */

package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	gc "github.com/untillpro/gochips"
)

var luckyRootPassword string
var luckyMonPassword string
var luckyNoAlerts bool
var luckySlackWebhookURL string
var luckySkipSSHConfiguration bool
var nodePrefix string

const (
	traefikLabelValue    = "traefik=true"
	prometheusLabelValue = "prometheus=true"
)

func alias(i int) string {
	return fmt.Sprintf("%s%d", nodePrefix, i)
}

var imluckyCmd = &cobra.Command{
	Use:   "imlucky IP [IP] [IP]",
	Short: "Builds swarm cluster with automatically assigned settings",
	Long:  "Specify one, two or three nodes to build cluster with automatically assigned settings",
	Args:  cobra.RangeArgs(1, 3),
	Run: loggedCmd(func(cmd *cobra.Command, args []string) {

		gc.ExitIfFalse(len(getNodesFromYml(getWorkingDir())) == 0, "Some nodes already has been added already. Use 'imlucky' command on clean configuration only")

		checkSSHAgent()

		clusterFile := unmarshalClusterYml()
		nodePrefix = clusterFile.ClusterNodeNamePrefix

		nodes := make(map[string]string)
		for i, arg := range args {
			nodeAlias := alias(i + 1)
			nodes[nodeAlias] = arg
		}

		AddNodes(nodes, luckyRootPassword, luckySkipSSHConfiguration)
		InstallDocker(false, []string{})
		if len(nodes) == 1 {
			AddToSwarm(true, []string{alias(1)})
			LabelAdd(alias(1), traefikLabelValue)
			LabelAdd(alias(1), prometheusLabelValue)
		} else if len(nodes) == 2 {
			AddToSwarm(true, []string{alias(1)})
			AddToSwarm(false, []string{alias(2)})
			LabelAdd(alias(2), traefikLabelValue)
			LabelAdd(alias(2), prometheusLabelValue)
		} else {
			AddToSwarm(true, []string{alias(1), alias(2), alias(3)})
			LabelAdd(alias(1), traefikLabelValue)
			LabelAdd(alias(2), prometheusLabelValue)
		}
		gc.Info("Current node labels:")
		LabelList()

		DeployTraefik(luckyMonPassword)
		DeploySwarmprom(luckyNoAlerts, luckySlackWebhookURL, luckyMonPassword, luckyMonPassword, luckyMonPassword)

		gc.Info("Done imlucky")

	}),
}
