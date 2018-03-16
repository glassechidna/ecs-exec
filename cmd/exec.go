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

	"github.com/spf13/cobra"
	"os/exec"
	"os"
	"strings"
	"github.com/spf13/viper"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/pkg/errors"
	"text/template"
	"bytes"
	"github.com/aws/aws-sdk-go/service/ec2"
)

var execCmd = &cobra.Command{
	Use:   "exec",
	Short: "Execute a command in a container running on ECS",
	Long: `
Specify a cluster (-c) and one of ECS service name (-s) or ECS task ID (-t). If 
a task ID is not provided, the first task (alphabetically by ID) is selected. 
Likewise, if a container name (--container) is not provided, the first one is
selected.

You can specify a pipe command (--pipe) that hooks up stdin/stdout to 'docker exec'
on the instance. There are three well-known commands that can be provided:

  $SSH   - the default. This does 'ssh ec2-user@{{.PrivateIpAddress}} {{.Command}}'
  $LKP   - uses LastKeypair (github.com/glassechidna/lastkeypair) to negotiate an
           SSH connection. Command invoked is 'ssh ec2-user@{{.InstanceArn}} {{.Command}}'
  $GOSSM - uses gossm (github.com/glassechidna/gossm) to invoke command using 
           AWS SSM RunCommand functionality. Doesn't support interactive usage.
           Invoked command is 'gossm -q -i {{.InstanceId}} -- {{.Command}}'

If you need a different way of connecting, you can provide your own command and
use the following placeholders:

  {{.InstanceArn}}      - EC2 instance ARN, e.g. arn:aws:ec2:us-east-1:11111111:instance/i-00001234abcdef
  {{.InstanceId}}       - EC2 instance id, e.g. i-00001234abcdef
  {{.PrivateIpAddress}} - EC2 private ip address
  {{.PublicIpAddress}}  - EC2 public ip address, if allocated
  {{.Command}}          - command that should be invoked on instance

`,
	Run: func(cmd *cobra.Command, args []string) {
		service, _ := cmd.Flags().GetString("service")
		task, _ := cmd.Flags().GetString("task")
		container, _ := cmd.Flags().GetString("container")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		cluster := viper.GetString("cluster")
		pipe := viper.GetString("pipe")
		command := strings.Join(args, " ")

		fullCommand := doExec(cluster, service, task, container, pipe, command)
		if dryRun {
			fmt.Println(fullCommand)
		} else {
			xplatExec(fullCommand)
		}
	},
}

var wellKnown = map[string]string{
	"@SSH": "ssh ec2-user@{{.PrivateIpAddress}} {{.Command}}",
	"@LKP": "ssh ec2-user@{{.InstanceArn}} {{.Command}}",
	"@GOSSM": "gossm -q -i {{.InstanceId}} -- {{.Command}}",
}

func expandPipeCommand(pipeCmd, dockerExecCmd string, instance *ec2.Instance, task *ecs.Task) string {
	wellKnownCmd := wellKnown[pipeCmd]
	if len(wellKnownCmd) > 0 {
		pipeCmd = wellKnownCmd
	}

	bits := strings.Split(*task.ContainerInstanceArn, ":")
	region := bits[3]
	accountId := bits[4]

	params := PipeParams{
		Command:          dockerExecCmd,
		InstanceId:       *instance.InstanceId,
		InstanceArn:      fmt.Sprintf("arn:aws:ec2:%s:%s:instance/%s", region, accountId, *instance.InstanceId),
		PrivateIpAddress: *instance.PrivateIpAddress,
	}

	if instance.PublicIpAddress != nil {
		params.PublicIpAddress = *instance.PublicIpAddress
	}

	tmpl, err := template.New("pipe").Parse(pipeCmd)
	if err != nil {
		panic(err)
	}

	var doc bytes.Buffer
	err = tmpl.Execute(&doc, params)
	return doc.String()
}

func doExec(cluster, service, taskId, container, pipe, cmd string) string {
	sess := cliSession()

	task, err := getTask(sess, cluster, service, taskId)
	if err != nil {
		panic(err)
	}

	instance, err := getInstance(sess, cluster, task)
	if err != nil {
		panic(err)
	}

	dockerExec := strings.Join(dockerExecCmd(*task.TaskArn, container, cmd), " ")
	return expandPipeCommand(pipe, dockerExec, instance, task)
}

func getInstance(sess *session.Session, cluster string, task *ecs.Task) (*ec2.Instance, error) {
	ecsApi := ecs.New(sess)
	ec2Api := ec2.New(sess)

	ecsResp, err := ecsApi.DescribeContainerInstances(&ecs.DescribeContainerInstancesInput{
		Cluster:            &cluster,
		ContainerInstances: []*string{task.ContainerInstanceArn},
	})
	if err != nil {
		return nil, err
	}
	instanceId := *ecsResp.ContainerInstances[0].Ec2InstanceId

	ec2Resp, err := ec2Api.DescribeInstances(&ec2.DescribeInstancesInput{InstanceIds: []*string{&instanceId}})
	if err != nil {
		return nil, err
	}

	return ec2Resp.Reservations[0].Instances[0], nil
}

func getTask(sess *session.Session, cluster, service, taskId string) (*ecs.Task, error) {
	api := ecs.New(sess)

	if len(service) > 0 {
		resp, err := api.ListTasks(&ecs.ListTasksInput{
			Cluster:     &cluster,
			ServiceName: &service,
		})
		if err != nil {
			return nil, err
		}
		taskId = "z"
		for _, task := range resp.TaskArns {
			if *task < taskId {
				taskId = *task
			}
		}
	} else if len(taskId) == 0 {
		return nil, errors.New("must provide at least one of ecs service or task id")
	}

	resp, err := api.DescribeTasks(&ecs.DescribeTasksInput{
		Cluster: &cluster,
		Tasks:   []*string{&taskId},
	})
	if err != nil {
		return nil, err
	}
	return resp.Tasks[0], nil
}

func xplatExec(command string) int {
	cmd := exec.Command("sh", "-c", command)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()
	return 0
}

func dockerExecCmd(taskArn, containerName, command string) []string {
	containerIdCmd := ""

	if len(containerName) > 0 {
		containerIdCmd = strings.Join([]string{
			"docker",
			"ps",
			"--filter label=com.amazonaws.ecs.task-arn=" + taskArn,
			"--filter label=com.amazonaws.ecs.container-name=" + containerName,
			"--format '{{.ID}}'",
		}, " ")
	} else {
		containerIdCmd = strings.Join([]string{
			"docker",
			"ps",
			"--filter label=com.amazonaws.ecs.task-arn=" + taskArn,
			`--format \'{{.Label \"com.amazonaws.ecs.container-name\"}} {{.ID}}\'`,
			`\| sort \| awk \'{print \$2}\' \| head -n1`,
		}, " ")
	}

	return []string{
		"docker",
		"exec",
		"-i",
		fmt.Sprintf(`$\(%s\)`, containerIdCmd),
		command,
	}
}

type PipeParams struct {
	InstanceArn      string
	InstanceId       string
	PrivateIpAddress string
	PublicIpAddress  string
	Command          string
}

func init() {
	RootCmd.AddCommand(execCmd)

	execCmd.Flags().StringP("service", "s", "", "ECS service - optional if task ID provided")
	execCmd.Flags().StringP("task", "t", "", "Task ID - defaults to first run by service if not provided")
	execCmd.Flags().String("container", "", "(Optional) Container name, defaults to first alphabetical")
	execCmd.Flags().Bool("dry-run", false, "Just prints command that would be run")

	execCmd.Flags().String("pipe", "$SSH", "Transport command that connects stdin/stdout to remote docker exec")
	viper.BindPFlag("pipe", execCmd.Flags().Lookup("pipe"))
}
