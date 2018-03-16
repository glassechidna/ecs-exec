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
	"github.com/spf13/cobra"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/spf13/viper"
	"github.com/glassechidna/ecs-exec/common"
	"github.com/aws/aws-sdk-go/service/ecs"
	"strings"
	"sort"
	"fmt"
	"github.com/fatih/color"
)

var lsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List all ECS services and/or tasks running on a cluster",
	Long: `
To list services, pass -s. To list task IDs, pass -t. To list services AND
tasks, pass -s -t. Finally, to also show container names pass --containers as
well.`,
	Run: func(cmd *cobra.Command, args []string) {
		showServices, _ := cmd.Flags().GetBool("services")
		showTasks, _ := cmd.Flags().GetBool("tasks")
		showContainers, _ := cmd.Flags().GetBool("containers")
		cluster := viper.GetString("cluster")
		ls(cluster, showServices, showTasks, showContainers)
	},
}

func describeAllTasks(api *ecs.ECS, input *ecs.DescribeTasksInput) ([]*ecs.Task, error) {
	allTaskArns := input.Tasks
	offset := 0

	allTasks := []*ecs.Task{}

	for {
		highIndex := offset + 100
		if highIndex > len(allTaskArns) {
			highIndex = len(allTaskArns)
		}
		if offset == highIndex {
			break
		}
		arnsPage := allTaskArns[offset:highIndex]
		input.Tasks = arnsPage
		offset = highIndex

		resp, err := api.DescribeTasks(input)
		if err != nil {
			return nil, err
		}

		allTasks = append(allTasks, resp.Tasks...)
	}

	return allTasks, nil
}

func allTasks(sess *session.Session, cluster string) []*ecs.Task {
	api := ecs.New(sess)

	taskArns := []*string{}
	err := api.ListTasksPages(&ecs.ListTasksInput{Cluster: &cluster}, func(page *ecs.ListTasksOutput, lastPage bool) bool {
		taskArns = append(taskArns, page.TaskArns...)
		return !lastPage
	})
	if err != nil {
		panic(err)
	}

	tasks, err := describeAllTasks(api, &ecs.DescribeTasksInput{Cluster: &cluster, Tasks: taskArns})
	if err != nil {
		panic(err)
	}

	sort.Sort(byService(tasks))
	return tasks
}

func ls(cluster string, showServices, showTasks, showContainers bool) {
	sess := cliSession()
	tasks := allTasks(sess, cluster)

	prevService := ""
	for _, task := range tasks {
		service := *task.Group
		service = service[len("service:"):]

		bits := strings.Split(*task.TaskArn, "/")
		taskId := bits[1]

		if showServices {
			if service != prevService {
				if !showTasks {
					fmt.Println(service) // no need to format if only showing services
				} else {
					color.New(color.Bold).Println(service)
				}
				prevService = service
			}
		}

		containers := []string{}
		for _, container := range task.Containers {
			containers = append(containers, *container.Name)
		}
		sort.Strings(containers)
		containersSuffix := ""
		if showContainers {
			containersSuffix = fmt.Sprintf(" (%s)", strings.Join(containers, ", "))
		}

		if showTasks {
			indentation := ""
			if showServices {
				indentation = "\t"
			}

			fmt.Printf("%s%s%s\n", indentation, taskId, containersSuffix)
		}

	}
}

type byService []*ecs.Task

func (s byService) Len() int {
	return len(s)
}
func (s byService) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
func (s byService) Less(i, j int) bool {
	a, b := s[i], s[j]
	cmp := strings.Compare(strings.ToLower(*a.Group), strings.ToLower(*b.Group))
	if cmp == 0 {
		return *a.TaskArn < *b.TaskArn
	} else {
		return cmp == -1
	}
}

func cliSession() *session.Session {
	profile := viper.GetString("profile")
	region := viper.GetString("region")
	return common.AwsSession(profile, region)
}

func init() {
	RootCmd.AddCommand(lsCmd)

	lsCmd.Flags().BoolP("services", "s", false, "Show services")
	lsCmd.Flags().BoolP("tasks", "t", false, "Show tasks")
	lsCmd.Flags().Bool("containers", false, "Show container names")
}
