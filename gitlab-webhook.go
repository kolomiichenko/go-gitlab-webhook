package main

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
)

//GitlabRepository represents repository information from the webhook
type GitlabRepository struct {
	Name, Url, Description, Home string
}

//Commit represents commit information from the webhook
type Commit struct {
	Id, Message, Timestamp, Url string
	Author                      Author
}

//Author represents author information from the webhook
type Author struct {
	Name, Email string
}

//Webhook represents push information from the webhook
type Webhook struct {
	Before, After, Ref, User_name string
	User_id, Project_id           int
	Repository                    GitlabRepository
	Commits                       []Commit
	Total_commits_count           int
}

//ConfigRepository represents a repository from the config file
type ConfigRepository struct {
	Name     string
	Commands []string
	Long     bool
	Branch   string
}

//Config represents the config file
type Config struct {
	Logfile      string
	ExecToStd    bool `json:"execToStd"`
	Address      string
	Port         int64
	Repositories []ConfigRepository
}

func PanicIf(err error, what ...string) {
	if err != nil {
		if len(what) == 0 {
			panic(err)
		}

		panic(errors.New(err.Error() + what[0]))
	}
}

var config Config
var configFile string

func main() {
	args := os.Args

	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGHUP)

	go func() {
		<-sigc
		config = loadConfig(configFile)
		log.Println("config reloaded")
	}()

	//if we have a "real" argument we take this as conf path to the config file
	if len(args) > 1 {
		configFile = args[1]
	} else {
		configFile = "config.json"
	}

	//load config
	config := loadConfig(configFile)

	//open log file
	writer, err := os.OpenFile(config.Logfile, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0666)
	PanicIf(err)

	//close logfile on exit
	defer func() {
		writer.Close()
	}()

	//setting logging output
	log.SetOutput(writer)

	//setting handler
	http.HandleFunc("/", hookHandler)

	address := config.Address + ":" + strconv.FormatInt(config.Port, 10)

	log.Println("Listening on " + address)

	//starting server
	err = http.ListenAndServe(address, nil)
	if err != nil {
		log.Println(err)
	}
}

func loadConfig(configFile string) Config {

	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGHUP)

	go func() {
		<-sigc
		config = loadConfig(configFile)
		log.Println("config reloaded")
	}()

	buffer, err := ioutil.ReadFile(configFile)
	PanicIf(err)

	err = json.Unmarshal(buffer, &config)
	PanicIf(err)

	return config
}

func hookHandler(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if r := recover(); r != nil {
			log.Println(r)
		}
	}()

	var hook Webhook

	//read request body
	var data, err = ioutil.ReadAll(r.Body)
	PanicIf(err, "while reading request")

	//unmarshal request body
	err = json.Unmarshal(data, &hook)
	PanicIf(err, "while unmarshaling request")

	refSlice := strings.Split(hook.Ref, "/")
	branch := refSlice[len(refSlice)-1]

	log.Println("hook.Ref: ", hook.Ref)
	log.Println("Branch: ", branch)

	//find matching config for repository name
	for _, repo := range config.Repositories {
		if repo.Name != hook.Repository.Name {
			continue
		}

		if len(repo.Branch) == 0 {
			repo.Branch = "master"
		}

		if repo.Branch != branch {
			continue
		}

		//execute commands for repository
		for _, cmd := range repo.Commands {
			log.Println("Webhook runned for project: " + repo.Name)
			if repo.Long {
				go execute(cmd)
			} else {
				execute(cmd)
			}

		}
	}
}

func execute(cmd string) {
	var command = exec.Command(cmd)
	out, err := command.Output()

	logger := log.New(os.Stdout, "[webhook] ", log.LstdFlags)

	if err != nil {
		if config.ExecToStd {
			logger.Println("Out before error: ", string(out))
			logger.Println("Error: ", err.Error())
		} else {
			log.Println("Out before error: ", string(out))
			log.Println("Error: ", err.Error())
		}
	} else {
		if config.ExecToStd {
			logger.Println("Executed: " + cmd)
			logger.Println("Output: " + string(out))
		} else {
			log.Println("Executed: " + cmd)
			log.Println("Output: " + string(out))
		}
	}
}
