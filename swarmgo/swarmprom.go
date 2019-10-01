/*
 * Copyright (c) 2018-present unTill Pro, Ltd. and Contributors
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 *
 */

package swarmgo

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	gc "github.com/untillpro/gochips"
)

const (
	swarmpromFolder          = "swarmprom"
	swarmpromComposeFileName = swarmpromFolder + "/swarmprom.yml"
	alertmanagerConfigPath   = swarmpromFolder + "/alertmanager/alertmanager.yml"
)

type infoForCopy struct {
	host   string
	client *SSHClient
}

var swarmpromCmd = &cobra.Command{
	Use:   "swarmprom",
	Short: "Create starter kit for swarm monitoring",
	Long:  `Deploys Prometheus, WebhookURL, cAdvisor, Node Exporter, Alert Manager and Unsee to the current swarm`,
	Run: loggedCmd(func(args []string) {
		firstEntry, clusterFile := getSwarmLeaderNodeAndClusterFile()
		if !firstEntry.node.Traefik {
			gc.Fatal("Need to deploy traefik before swarmprom deploy")
		}
		deploySwarmprom(clusterFile, firstEntry)
	}),
}

func deploySwarmprom(clusterFile *clusterFile, firstEntry *entry) {
	client := getSSHClient(clusterFile)
	clusterFile.GrafanaPassword = readPasswordPrompt("Grafana admin user password")
	gc.Info("Enter webhook URL for slack channel", clusterFile.ChannelName)
	clusterFile.WebhookURL = waitUserInput()
	//TODO don't forget to implement passwords for prometheus and traefik
	host := firstEntry.node.Host
	forCopy := infoForCopy{
		host, client,
	}
	gc.Info("Trying to install dos2unix")
	client.ExecOrExit(host, "sudo apt-get install dos2unix")
	curDir := getSourcesDir()
	copyToHost(&forCopy, filepath.ToSlash(filepath.Join(curDir, swarmpromFolder)))
	filesToApplyTemplate := [2]string{alertmanagerConfigPath, swarmpromComposeFileName}
	for _, fileToApplyTemplate := range filesToApplyTemplate {
		appliedBuffer := executeTemplateToFile(fileToApplyTemplate, clusterFile)
		client.ExecOrExit(host, "cat > ~/"+fileToApplyTemplate+" << EOF\n\n"+
			appliedBuffer.String()+"\nEOF")
		gc.Info(fileToApplyTemplate, "applied by template")
	}
	gc.Info("Trying to deploy swarmprom")
	client.ExecOrExit(host, "sudo docker stack deploy -c "+swarmpromComposeFileName+" prom")
	gc.Info("Swarmprom deployed")
	gc.ExitIfError(postTestMessageToAlertmanager(clusterFile.WebhookURL, clusterFile.ChannelName))
}

func copyToHost(forCopy *infoForCopy, src string) {
	info, err := os.Lstat(src)
	gc.ExitIfError(err)
	if info.IsDir() {
		copyDirToHost(src, forCopy)
	} else {
		copyFileToHost(src, forCopy)
	}
}

func copyDirToHost(dirPath string, forCopy *infoForCopy) {
	forCopy.client.ExecOrExit(forCopy.host, "mkdir -p "+substringAfter(dirPath,
		filepath.ToSlash(getSourcesDir())+"/"))
	dirContent, err := ioutil.ReadDir(dirPath)
	gc.ExitIfError(err)
	for _, dirEntry := range dirContent {
		src := filepath.ToSlash(filepath.Join(dirPath, dirEntry.Name()))
		copyToHost(forCopy, src)
	}
}

func copyFileToHost(filePath string, forCopy *infoForCopy) {
	relativePath := substringAfter(filePath, filepath.ToSlash(getSourcesDir())+"/")
	err := forCopy.client.CopyPath(forCopy.host, filePath, relativePath)
	gc.ExitIfError(err)
	forCopy.client.ExecOrExit(forCopy.host, "sudo dos2unix "+relativePath)
	forCopy.client.ExecOrExit(forCopy.host, "sudo chown root:root "+relativePath)
	forCopy.client.ExecOrExit(forCopy.host, "sudo chmod 777 "+relativePath)
	gc.ExitIfError(err)
	gc.Info(relativePath, "copied on host")
}

func postTestMessageToAlertmanager(URL, channelName string) error {
	jsonMap := map[string]string{"channel": channelName, "username": "alertmanager", "text": "Alertmanager successfully installed to cluster", "icon_emoji": ":ghost:"}
	jsonEntry, _ := json.Marshal(jsonMap)
	_, err := http.Post(URL, "application/json", bytes.NewReader(jsonEntry))
	return err
}
