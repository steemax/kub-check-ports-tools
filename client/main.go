package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-ping/ping"
	"github.com/gorilla/mux"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type Client struct {
	Name     string
	Status   string
	LastSeen time.Time
	Active   bool
	Tasks    []string
}

const (
	serverAddress   = "http://check-port.check.svc:8080"
	registerTimeout = 7 * time.Second
)

func registerClient(clientName string) error {
	url := fmt.Sprintf("%s/connect/clients?name=%s", serverAddress, clientName)

	// Send the request to register the client
	response, err := http.Get(url)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	// Read the response body
	_, err = ioutil.ReadAll(response.Body)
	if err != nil {
		return err
	}

	return nil
}

func updateClientStatus(clientName string) error {
	url := fmt.Sprintf("%s/update_last_seen?name=%s", serverAddress, clientName)

	// Send the request to update the client status
	response, err := http.Get(url)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	// Read the response body
	_, err = ioutil.ReadAll(response.Body)
	if err != nil {
		return err
	}

	return nil
}

func getTasks(clientName string) ([]string, error) {
	url := fmt.Sprintf("%s/get_tasks?name=%s", serverAddress, clientName)

	// Send the GET request
	response, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	// Read the response body
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	// Parse the JSON array of tasks
	var tasks []string
	err = json.Unmarshal(body, &tasks)
	if err != nil {
		return nil, err
	}

	return tasks, nil
}

func submitResult(clientName, checkMethod, address, port, result string) error {
	url := fmt.Sprintf("%s/results_from_clients", serverAddress)

	// Create a JSON payload with the client name, check method, address, port, and result
	payload := map[string]interface{}{
		"clientName":  clientName,
		"checkMethod": checkMethod,
		"address":     address,
		"port":        port,
		"result":      result,
		"time":        time.Now(),
	}
	payloadBytes, err := json.Marshal(payload)

	for key, value := range payload {
		fmt.Printf("%s:%s\n", key, value)
	}
	if err != nil {
		fmt.Printf("Can't marshal JSON payload: %v\n", err)
		return err
	}
	// Преобразуйте байтовый массив в строку
	payloadString := string(payloadBytes)

	// Напечатайте JSON
	fmt.Println(payloadString)

	// Send the POST request with the payload
	_, err = http.Post(url, "application/json", bytes.NewBuffer(payloadBytes))
	if err != nil {
		return err
	}
	time.Sleep(time.Second) // Подождать 1 секунду перед выводом результатов

	return nil
}
func getNamespaceFromAPI() (string, error) {
	// Читаем файл с информацией о текущем поде
	data, err := ioutil.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		return "", err
	}

	// Возвращаем имя неймспейса в виде строки
	return string(data), nil
}

func resolveHandler(w http.ResponseWriter, r *http.Request) {
	log.Print("Start resolve domain Name from kube-diag application...")
	domain := r.URL.Query().Get("domain")
	if domain == "" {
		http.Error(w, "Domain not specified", http.StatusBadRequest)
		return
	}

	addresses, err := net.LookupHost(domain)
	if err != nil {
		http.Error(w, "Failed to resolve domain: "+err.Error(), http.StatusInternalServerError)
		return
	}

	jsonResponse, err := json.Marshal(addresses)
	if err != nil {
		http.Error(w, "Failed to encode response: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonResponse)
}

func main() {
	r := mux.NewRouter()
	r.HandleFunc("/resolve", resolveHandler)
	go func() { // запускаем веб сервер в фоне отдельным процессом, чтобы продолжить выполнение остального кода
		err := http.ListenAndServe(":8080", r)
		if err != nil {
			log.Println("Cannot open port", err)
		} else {
			log.Print("Listening on port :8080 for kube-diag app")
		}
	}()
	var clientName string
	if os.Getenv("KUBERNETES_SERVICE_HOST") == "" {
		fmt.Println("Application is not running inside Kubernetes cluster")
		clientNameLeg, err := os.Hostname()
		clientName = clientNameLeg
		if err != nil {
			fmt.Println("Failed to get client name:", err)
			return
		} else {
			fmt.Println("Client name:", clientName)
		}
		for {
			err = registerClient(clientName)
			if err == nil {
				// Регистрация прошла успешно
				break
			}

			fmt.Println("Failed to register client:", err)
			time.Sleep(time.Second) // Подождать некоторое время перед повторной попыткой
		}
	} else {
		// Создаем конфигурацию клиента Kubernetes API
		config, err := rest.InClusterConfig()
		if err != nil {
			fmt.Println("Failed to create Kubernetes client config:", err)
			return
		}

		// Создаем клиент Kubernetes API
		clientset, err := kubernetes.NewForConfig(config)
		if err != nil {
			fmt.Println("Failed to create Kubernetes client:", err)
			return
		}

		// Получаем имя текущего пода
		podName, err := os.Hostname()
		if err != nil {
			fmt.Println("Failed to get pod name:", err)
			return
		}
		// Получаем имя неймспейса текущего пода
		namespace, err := getNamespaceFromAPI()
		if err != nil {
			fmt.Println("Failed to get namespace from API:", err)
			return
		}

		// Создаем контекст
		ctx := context.TODO()

		// Получаем информацию о текущем поде
		pod, err := clientset.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
		if err != nil {
			fmt.Println("Failed to get pod information:", err)
			return
		}

		// Получаем имя узла, на котором запущен под
		nodeName := pod.Spec.NodeName
		fmt.Println("Node name:", nodeName)
		clientName = nodeName
		fmt.Println("Client name:", clientName)
		for {
			err = registerClient(clientName)
			if err == nil {
				// Регистрация прошла успешно
				break
			}

			fmt.Println("Failed to register client:", err)
			time.Sleep(time.Second) // Подождать некоторое время перед повторной попыткой
		}
	}
	// Start the periodic update loop
	go func() {
		for {
			// Update the client status
			err := updateClientStatus(clientName)
			if err != nil {
				fmt.Println("Failed to update client status:", err)
				fmt.Printf("Try again register client: %v", clientName)
				for {
					err = registerClient(clientName)
					if err == nil {
						// Регистрация прошла успешно
						break
					}

					fmt.Println("Failed to register client:", err)
					time.Sleep(time.Second) // Подождать некоторое время перед повторной попыткой
				}
			}

			// Wait for some time before updating again
			time.Sleep(registerTimeout)
		}
	}()

	// Start the task retrieval loop
	go func() {
		for {
			// Retrieve tasks from the server
			tasks, err := getTasks(clientName)
			if err != nil {
				fmt.Println("Failed to retrieve tasks:", err)
				for {
					err = registerClient(clientName)
					if err == nil {
						// Регистрация прошла успешно
						break
					}

					fmt.Println("Failed to register client:", err)
					time.Sleep(time.Second) // Подождать некоторое время перед повторной попыткой
				}
			}

			// Process tasks
			for _, task := range tasks {
				// Parse the task into its components
				taskParts := strings.Split(task, ":")
				if len(taskParts) != 3 {
					fmt.Println("Invalid task format")
					continue
				}

				checkMethod := taskParts[0]
				address := taskParts[1]
				port := taskParts[2]

				// Execute the task and get the result
				result := executeTask(clientName, task)

				// Submit the result to the server
				err := submitResult(clientName, checkMethod, address, port, result)
				if err != nil {
					fmt.Println("Failed to submit result:", err)
				}
			}

			// Wait for some time before retrieving tasks again
			time.Sleep(5 * time.Second)
		}
	}()

	// Keep the client running indefinitely
	select {}
}

func executeTask(clientName string, task string) string {
	// Parse the task into its components
	taskParts := strings.Split(task, ":")
	if len(taskParts) != 3 {
		fmt.Println("Invalid task format")
		return ""
	}

	checkMethod := taskParts[0]
	address := taskParts[1]
	port := taskParts[2]

	if checkMethod == "tcp" {
		// Perform TCP check (TCP connectivity check)
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%s", address, port), time.Second)
		if err != nil {
			fmt.Println("Not Available")
			return "Not Available"
		}
		defer conn.Close()
		fmt.Println("Available")
		return "Available"
	} else if checkMethod == "ping" {
		// Perform ping check
		pinger, err := ping.NewPinger(address)
		if err != nil {
			fmt.Println("Failed to create pinger:", err)
			return "Not Available"
		}

		pinger.Count = 1             // Send only one packet
		pinger.Timeout = time.Second // Set timeout to 1 second

		err = pinger.Run() // Execute the ping
		if err != nil {
			fmt.Println("Ping failed:", err)
			return "Not Available"
		}

		stats := pinger.Statistics() // Get ping statistics

		if stats.PacketsRecv > 0 {
			return "Available"
		} else {
			return "Not Available"
		}
	} else if checkMethod == "udp" {
		// Perform UDP check
		conn, err := net.Dial("udp", fmt.Sprintf("%s:%s", address, port))
		if err != nil {
			fmt.Println("Not Available")
			return "Not Available"
		}
		defer conn.Close()
		fmt.Println("Available")
		return "Available"
	} else {
		fmt.Println("Task execution not implemented")
		return ""
	}
}
