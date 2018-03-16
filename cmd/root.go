// Copyright © 2018 Aidan Steele <aidan.steele@glassechidna.com.au>
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

package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string

var RootCmd = &cobra.Command{
	Use:   "ecs-exec",
	Long: `
ecs-exec runs commands in containers on AWS Elastic Container Service and
connects stdin/stdout to your terminal. This is useful for running ad-hoc
commands or connecting debuggers from your IDE.

ecs-exec relies on external 'pipe' commands to provide the underlying 
connectivity to 'docker exec' invocations on the remote instance. Built-in
support for standard SSH, LastKeypair-negotiated SSH and SSM RunCommand is
provided. Custom commands can also be used, see ecs-exec exec --help for 
details.
`,
}

// Execute adds all child commands to the root command sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(-1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	RootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.ecs-exec.yaml)")
	RootCmd.PersistentFlags().StringP("cluster", "c", "default", "Name of ECS cluster")
	RootCmd.PersistentFlags().StringP("profile", "p", "", "(Optional) profile name in ~/.aws/config to use")
	RootCmd.PersistentFlags().StringP("region", "r", "", "(Optional) AWS region of cluster")

	viper.BindPFlags(RootCmd.PersistentFlags())
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" { // enable ability to specify config file via flag
		viper.SetConfigFile(cfgFile)
	}

	viper.SetConfigName(".ecs-exec") // name of config file (without extension)
	viper.AddConfigPath("$HOME")  // adding home directory as first search path
	viper.AutomaticEnv()          // read in environment variables that match
	viper.ReadInConfig()
}
