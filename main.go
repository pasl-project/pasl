/*
PASL - Personalized Accounts & Secure Ledger

Copyright (C) 2018 PASL Project

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/modern-go/concurrent"
	"github.com/urfave/cli"

	"github.com/pasl-project/pasl/api"
	"github.com/pasl-project/pasl/blockchain"
	"github.com/pasl-project/pasl/crypto"
	"github.com/pasl-project/pasl/defaults"
	"github.com/pasl-project/pasl/network"
	"github.com/pasl-project/pasl/network/pasl"
	"github.com/pasl-project/pasl/storage"
	"github.com/pasl-project/pasl/utils"
)

var p2pPortFlag cli.UintFlag = cli.UintFlag{
	Name:  "p2p-bind-port",
	Usage: "P2P bind port",
	Value: uint(defaults.P2PPort),
}
var dataDirFlag cli.StringFlag = cli.StringFlag{
	Name:  "data-dir",
	Usage: "Directory to store blockchain files",
}

func run(ctx *cli.Context) error {
	utils.Tracef(defaults.UserAgent)

	dataDir := ctx.GlobalString(dataDirFlag.GetName())
	if dataDir == "" {
		var err error
		if dataDir, err = utils.GetDataDir(); err != nil {
			return fmt.Errorf("Failed to obtain valid data directory path. Use %s flag to manually specify data directory location. Error: %v", dataDirFlag.GetName(), err)
		}
	}

	if err := utils.CreateDirectory(&dataDir); err != nil {
		return fmt.Errorf("Failed to create data directory %v", err)
	}
	dbFileName := filepath.Join(dataDir, "storage.db")
	err := storage.WithStorage(&dbFileName, defaults.AccountsPerBlock, func(storage storage.Storage) error {
		blockchain, err := blockchain.NewBlockchain(storage)
		if err != nil {
			return err
		}

		config := network.Config{
			ListenAddr:     fmt.Sprintf("%s:%d", defaults.P2PBindAddress, ctx.GlobalUint(p2pPortFlag.GetName())),
			MaxIncoming:    defaults.MaxIncoming,
			MaxOutgoing:    defaults.MaxOutgoing,
			TimeoutConnect: defaults.TimeoutConnect,
		}

		key, err := crypto.NewKey(crypto.NIDsecp256k1)
		if err != nil {
			return err
		}
		nonce := utils.Serialize(key.Public)

		peerUpdates := make(chan pasl.PeerInfo)
		return pasl.WithManager(nonce, blockchain, peerUpdates, defaults.TimeoutRequest, func(manager network.Manager) error {
			return network.WithNode(config, manager, func(node network.Node) error {
				for _, hostPort := range strings.Split(defaults.BootstrapNodes, ",") {
					node.AddPeer("tcp", hostPort)
				}

				updatesListener := concurrent.NewUnboundedExecutor()
				updatesListener.Go(func(ctx context.Context) {
					for {
						select {
						case peer := <-peerUpdates:
							utils.Tracef("   %s:%d last seen %s ago", peer.Host, peer.Port, time.Since(time.Unix(int64(peer.LastConnect), 0)))
							node.AddPeer("tcp", fmt.Sprintf("%s:%d", peer.Host, peer.Port))
						case <-ctx.Done():
							return
						}
					}
				})
				defer updatesListener.StopAndWaitForever()

				return network.WithRpcServer(fmt.Sprintf("%s:%d", defaults.RPCBindAddress, defaults.RPCPort), api.NewApi(blockchain), func() error {
					c := make(chan os.Signal, 2)
					signal.Notify(c, os.Interrupt, syscall.SIGTERM)
					<-c
					utils.Tracef("Exit signal received. Terminating...")
					return nil
				})
			})
		})
	})
	if err != nil {
		return fmt.Errorf("Failed to initialize storage. %v", err)
	}
	return nil
}

func main() {
	app := cli.NewApp()
	app.Usage = "PASL command line interface"
	app.Version = defaults.UserAgent
	app.Action = run
	app.Flags = []cli.Flag{
		p2pPortFlag,
		dataDirFlag,
	}
	app.CommandNotFound = func(c *cli.Context, command string) {
		cli.ShowAppHelp(c)
		os.Exit(1)
	}
	if err := app.Run(os.Args); err != nil {
		utils.Panicf("Error running application: %v", err)
		os.Exit(2)
	}
	os.Exit(0)
}
