/*
 * Copyright (c) 2018-present unTill Pro, Ltd. and Contributors
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 *
 */

package cmd

import (
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh"
	"io/ioutil"
	"log"
	"strings"
	"fmt"
	"gopkg.in/yaml.v2"
	"path/filepath"
)

type SwarmMode int

const (
	nodesFileName           = "nodes.yml"
	worker        SwarmMode = iota
	manager
	leader
)

type user struct {
	host, alias, userName, passToRoot, passToUser string
}
type Node struct {
	Host, Alias, DockerVersion string
	SwarmMode                  SwarmMode
}

// addNodeCmd represents the addNode command
var addNodeCmd = &cobra.Command{
	Use:   "add",
	Short: "add <Alias1>=<1IP> <Alias2>=<IP2>",
	Long:  `Add node with specified alias and current IP to cluster and config access with keys`,
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		readFileIfExists(clusterFileName, "Need to use swarmgo init first!")
		publicKeyFile, privateKeyFile := findSshKeys()
		fmt.Println("Enter password to crypt/decrypt you private key")
		passToKey := waitUserInput()
		if !checkFileExistence(publicKeyFile) && !checkFileExistence(privateKeyFile) {
			bitSize := 4096
			err := generateKeysAndWriteToFile(bitSize, privateKeyFile, publicKeyFile, passToKey)
			CheckErr(err)
		} else {
			log.Println("Keys already exist")
		}
		log.Println("Checking if node already configured to use keys")
		nodesFromYaml := getNodesFromYml(getCurrentDir())
		hosts := make([]string, len(nodesFromYaml))
		for index, node := range nodesFromYaml {
			hosts[index] = node.Alias + "=" + node.Host
		}
		i := 0
		for _, arg := range args {
			if contains(hosts, arg) {
				log.Println(arg + " node already configured to use keys!")
			} else {
				args[i] = arg
				i++
			}
		}
		args = args[:i]
		if len(args) == 0 {
			log.Fatal("All passed hosts already configured to use keys")
		}
		users := make([]user, len(args))
		for index, arg := range args {
			var user user
			userAndAlias := strings.Split(arg, "=")
			user.alias = userAndAlias[0]
			user.host = userAndAlias[1]
			fmt.Println("input user name for host " + user.host)
			for len(user.userName) == 0 {
				fmt.Println("User name can't be empty!")
				user.userName = waitUserInput()
			}
			fmt.Println("input password for root user of " + user.host)
			user.passToRoot = waitUserInput()
			fmt.Println("input password for new user of " + user.host)
			user.passToUser = waitUserInput()
			users[index] = user
		}
		nodesChannel := make(chan Node)
		for _, value := range users {
			go func(user user) {
				//passToRoot to user and key from input
				configHostToUseKeys(user, publicKeyFile, privateKeyFile, passToKey)
				node := Node{
					Host:  user.host,
					Alias: user.alias,
				}
				nodesChannel <- node
			}(value)
		}
		nodes := make([]Node, 0, len(args))
		for range args {
			nodes = append(nodes, <-nodesChannel)
		}
		close(nodesChannel)
		marshaledNode, err := yaml.Marshal(&nodes)
		CheckErr(err)
		nodesFile := filepath.Join(getCurrentDir(), nodesFileName)
		err = ioutil.WriteFile(nodesFile, marshaledNode, 0600)
		CheckErr(err)
	},
}

func configHostToUseKeys(user user, publicKeyFile, privateKeyFile, passToKey string) {
	host := user.host
	userName := user.userName
	logWithPrefix(host, "Host "+host)
	logWithPrefix(host, "Connecting to remote servers root with password..")
	sshConfig := &ssh.ClientConfig{
		User:            "root",
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Auth:            []ssh.AuthMethod{ssh.Password(user.passToRoot)},
	}
	execSshCommand(host, "adduser --disabled-password --gecos \"\" "+userName, sshConfig)
	logWithPrefix(host, "New user "+userName+" added")
	execSshCommand(host, "echo \""+userName+":"+user.passToUser+"\" | sudo chpasswd", sshConfig)
	execSshCommand(host, "usermod -aG sudo "+userName, sshConfig)
	logWithPrefix(host, "Sudo permissions given to "+userName)
	sshConfig = &ssh.ClientConfig{
		User:            userName,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Auth:            []ssh.AuthMethod{ssh.Password(user.passToUser)},
	}
	logWithPrefix(host, "Relogin from root to "+userName)
	sudoExecSshCommand(host, "passwd -l root", sshConfig)
	logWithPrefix(host, "Root password disabled")
	execSshCommand(host, "mkdir ~/.ssh", sshConfig)
	execSshCommand(host, "chmod 700 ~/.ssh", sshConfig)
	execSshCommand(host, "touch ~/.ssh/authorized_keys", sshConfig)
	execSshCommand(host, "chmod 600 ~/.ssh/authorized_keys", sshConfig)
	//read public key
	pemBytes, err := ioutil.ReadFile(publicKeyFile)
	CheckErr(err)
	execSshCommand(host, "echo \""+string(pemBytes)+"\" | tee ~/.ssh/authorized_keys", sshConfig)
	logWithPrefix(host, "Host public key added to remote server")
	sudoExecSshCommand(host, "sed -i 's/#PasswordAuthentication yes/PasswordAuthentication no/g' /etc/ssh/sshd_config",
		sshConfig)
	logWithPrefix(host, host+" password auth disabled")
	sshConfig = initSshConnectionConfigWithPublicKeys(userName, privateKeyFile, passToKey)
	sudoExecSshCommand(host, "ufw allow OpenSSH", sshConfig)
	sudoExecSshCommand(host, "yes | sudo ufw enable", sshConfig)
	sudoExecSshCommand(host, "ufw reload", sshConfig)
	logWithPrefix(host, "Firewall reloaded to work with OpenSSH")
}

func init() {
	nodeCmd.AddCommand(addNodeCmd)
}
