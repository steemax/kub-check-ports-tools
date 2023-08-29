package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Client struct {
	Name       string
	Status     string
	LastSeen   time.Time
	Active     bool
	Tasks      []string
	TaskStatus string
}

type CheckMethod string

type CheckResult struct {
	ClientName  string
	CheckMethod string
	Address     string
	Port        string
	Result      string
	Time        time.Time
}

const (
	inactivityTimeout = 180 * time.Minute
	clientTimeout     = 10 * time.Second
	refreshRate       = 5 * time.Second
	taskTimeout       = 2 * time.Minute
)

var connectedClients = make(map[string]*Client)
var checkResults = make(map[string]CheckResult)
var clientStatus = make(map[string]string)
var disconnectedClients = make([]string, 0)
var disconnectedClientsMutex sync.Mutex
var clientStatusMutex sync.Mutex
var checkResultsMutex sync.Mutex
var tasksMutex sync.Mutex

func mainPageHandler(w http.ResponseWriter, r *http.Request) {

	content := `<div class="container">
    <div class="left-content">
        <h3>Select check method:</h3>
        <form action="/submit" method="POST">
            <select name="checkMethod" style="width: 100px;">
                <option value="tcp">TCP</option>
                <option value="udp">UDP</option>
                <option value="ping">Ping</option>
            </select>
            <br>
            <div style="margin-bottom: 10px;"></div> <!-- Добавьте отступ -->
            <label for="address" style="display: inline-block; width: 120px;">Адрес проверки:</label>
            <input type="text" id="address" name="address" style="width: 400px;"><br>
            <label for="port" style="display: inline-block; width: 120px;">Порт:</label>
            <input type="text" id="port" name="port" style="width: 400px;"><br>
            <div style="margin-bottom: 20px;"></div> <!-- Добавьте отступ -->
            <input type="submit" value="Submit" style="font-size: 16px; width: 150px;">
			</form>
			<form></form>
        <form id="clearForm" method="POST">
			<button type="button" onclick="refreshPage()" style="font-size: 16px; width: 150px;">Refresh</button>
    		<button type="button" onclick="submitForm('/clear_all_task')" style="font-size: 16px; width: 150px;">Clear Tasks</button>
    		<button type="button" onclick="submitForm('/clear_all_result')" style="font-size: 16px; width: 150px;">Clear Result</button>
    		<input id="actionInput" type="hidden" name="action">
		</form>
		<script>
			function submitForm(url) {
				document.getElementById("clearForm").action = url;
				document.getElementById("clearForm").submit();
			}

			function refreshPage() {
				location.reload();
			}
		</script>
    </div>
    <div class="main-content">Connected Clients:</div>
    <div class="client-info">`

	// Add JavaScript function for refreshing the page
	fmt.Fprint(w, `<script>
    function refreshPage() {
        location.reload();
    }
</script>`)

	// Write the content to the response
	fmt.Fprint(w, `<style>
		        .container { display: flex; }
		        .left-content { flex: 1; }
		        .main-content { flex: 1; }
		        .client-info { flex: 1; text-align: right; }
		        .client { margin-bottom: 10px; }
		        .client-name { font-weight: bold; }
		        .client-status { font-weight: bold; }
		        .red { color: red; }
		        .green { color: green; }
		    </style>`)
	fmt.Fprint(w, content)

	// Display active clients as a table
	fmt.Fprint(w, "<h3>Active Clients:</h3>")
	fmt.Fprint(w, "<table>")
	fmt.Fprint(w, "<tr><th>Name</th><th>Status</th><th>Last Seen</th></tr>")
	for name, client := range connectedClients {
		statusClass := "red"
		if clientStatus[name] == "Connected" {
			statusClass = "green"
		}
		fmt.Fprintf(w, "<tr><td>%s</td><td class=\"%s\">%s</td><td>%s</td></tr>", client.Name, statusClass, clientStatus[name], client.LastSeen.Format("2006-01-02 15:04:05"))
	}
	fmt.Fprint(w, "</table>")
	// Add CSS styling for the active clients table
	fmt.Fprint(w, `<style>
table {
    border-collapse: collapse;
    width: 100%;
}

th, td {
    border: 1px solid black;
    padding: 8px;
    text-align: left;
}

th {
    background-color: #f2f2f2;
}

.green {
    color: green;
}

.red {
    color: red;
}
</style>`)

	// Display active tasks as a table
	fmt.Fprint(w, "<h3>Active Tasks:</h3>")
	fmt.Fprint(w, "<table>")
	fmt.Fprint(w, "<tr><th>Client</th><th>Tasks</th><th>Status</th></tr>")
	for _, client := range connectedClients {
		if len(client.Tasks) > 0 {
			fmt.Fprintf(w, "<tr><td>%s</td><td>%s</td><td>%s</td></tr>", client.Name, strings.Join(client.Tasks, "<br>"), client.TaskStatus)
		}
	}
	fmt.Fprint(w, "</table>")
	// Add CSS styling for the active tasks table
	fmt.Fprint(w, `<style>
table {
    border-collapse: collapse;
    width: 100%;
}

th, td {
    border: 1px solid black;
    padding: 8px;
    text-align: left;
}

th {
    background-color: #f2f2f2;
}
</style>`)
	// Display submitted check results as a table
	fmt.Fprint(w, "<h3>Check Results:</h3>")
	fmt.Fprint(w, "<table>")
	fmt.Fprint(w, "<tr><th>Client</th><th>Method</th><th>Address</th><th>Port</th><th>Result</th><th>Time</th></tr>")
	for clientName, result := range checkResults {
		resultColor := "red"
		if result.Result == "Available" {
			resultColor = "green"
		}
		fmt.Fprintf(w, "<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td class=\"%s\">%s</td><td>%s</td></tr>", clientName, result.CheckMethod, result.Address, result.Port, resultColor, result.Result, result.Time.Format("2006-01-02 15:04:05"))
	}
	fmt.Fprint(w, "</table>")

	// Add CSS styling for the table
	fmt.Fprint(w, `<style>
	table {
		border-collapse: collapse;
		width: 100%;
	}

	th, td {
		border: 1px solid black;
		padding: 8px;
		text-align: left;
	}

	th {
		background-color: #f2f2f2;
	}
	.red {
		color: red;
	}

	.green {
		color: green;
	}
</style>`)
}

func clientsPageHandler(w http.ResponseWriter, r *http.Request) {
	// Get the client name from the request parameters
	clientName := r.URL.Query().Get("name")

	// Create a new client and set the initial status and tasks
	client := &Client{
		Name:     clientName,
		Status:   "Connected",
		LastSeen: time.Now(),
		Active:   true,
		Tasks:    []string{}, // Initialize tasks as an empty slice
	}

	// Add the client to the connected clients map
	connectedClients[clientName] = client

	// Write a response to confirm the client connection
	fmt.Fprintf(w, "Client %s connected successfully", clientName)
}

func updateLastSeenHandler(w http.ResponseWriter, r *http.Request) {
	// Get the client name from the request parameters
	clientName := r.URL.Query().Get("name")

	// Check if the client exists in the connected clients map
	if client, ok := connectedClients[clientName]; ok {
		// Update the last seen time of the client to the current time
		client.LastSeen = time.Now()

		// Write a response to confirm the successful update
		fmt.Fprintf(w, "Last seen time updated for client %s", clientName)
	} else {
		// Write a response to indicate that the client was not found
		fmt.Fprintf(w, "Client %s not found", clientName)
	}
}

func checkClientActivity() {
	for {
		time.Sleep(clientTimeout)

		clientStatusMutex.Lock()
		for name, client := range connectedClients {
			if time.Since(client.LastSeen) > clientTimeout {
				clientStatus[name] = "Disconnected"
				client.Active = false
				disconnectedClients = append(disconnectedClients, name)

			} else {
				clientStatus[name] = "Connected"
				client.Active = true
				removeDisconnectedClient(name)
			}
		}
		clientStatusMutex.Unlock()
	}
}

func clearTaskPeriodically() {
	for {
		time.Sleep(taskTimeout)

		tasksMutex.Lock()

		for _, client := range connectedClients {
			client.Tasks = []string{} // Установить пустой срез задач
		}
		tasksMutex.Unlock()
	}
}

func getTasksHandler(w http.ResponseWriter, r *http.Request) {
	// Get the client name from the request parameters
	clientName := r.URL.Query().Get("name")

	// Get the client
	client, ok := connectedClients[clientName]
	if !ok {
		http.Error(w, "Client not found", http.StatusNotFound)
		return
	}

	// Marshal the client's tasks to JSON
	tasksJSON, err := json.Marshal(client.Tasks)
	if err != nil {
		http.Error(w, "Failed to marshal tasks", http.StatusInternalServerError)
		return
	}

	// Set the response content type and write the tasks JSON
	w.Header().Set("Content-Type", "application/json")
	w.Write(tasksJSON)
}

func submitHandler(w http.ResponseWriter, r *http.Request) {
	// Retrieve the form values
	checkMethod := r.FormValue("checkMethod")
	address := r.FormValue("address")
	port := r.FormValue("port")

	// Generate tasks for every active client
	if checkMethod == "tcp" || checkMethod == "udp" {
		if len(address) < 4 {
			fmt.Printf("address is empty")
		} else if len(port) < 1 {
			fmt.Printf("address is empty")
		} else {
			for _, client := range connectedClients {
				if client.Active {
					// Build the task string
					task := fmt.Sprintf("%s:%s:%s", checkMethod, address, port)

					// Add the task to the client's tasks array
					client.Tasks = append(client.Tasks, task)
					client.TaskStatus = "Active"
				}
			}
		}

	} else if checkMethod == "ping" {
		if len(address) < 4 {
			fmt.Printf("address is empty")
		} else {
			for _, client := range connectedClients {
				if client.Active {
					// Build the task string
					task := fmt.Sprintf("%s:%s:%s", checkMethod, address, port)

					// Add the task to the client's tasks array
					client.Tasks = append(client.Tasks, task)
					client.TaskStatus = "Active"
				}
			}
		}
	}

	// Redirect back to the main page
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func clearAllTasksHandler(w http.ResponseWriter, r *http.Request) {
	tasksMutex.Lock()
	defer tasksMutex.Unlock()

	for _, client := range connectedClients {
		client.Tasks = []string{} // Установить пустой срез задач
	}

	// Redirect back to the main page
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func clearAllResultsHandler(w http.ResponseWriter, r *http.Request) {
	checkResultsMutex.Lock()
	defer checkResultsMutex.Unlock()

	checkResults = make(map[string]CheckResult) // Установить пустой срез результатов

	// Redirect back to the main page
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func resultsFromClientsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		fmt.Println("Method not allowed for resultsFromClientsHandler")
		return
	}

	// Parse the JSON payload
	var payload map[string]string
	err := json.NewDecoder(r.Body).Decode(&payload)
	if err != nil {
		http.Error(w, "Failed to parse JSON payload", http.StatusBadRequest)
		fmt.Println("Failed to parse JSON payload")
		return
	}

	// Retrieve the values from the payload
	clientName := payload["clientName"]
	checkMethod := payload["checkMethod"]
	address := payload["address"]
	port := payload["port"]
	result := payload["result"]

	// Create an instance of CheckResult
	checkResult := CheckResult{
		ClientName:  clientName,
		CheckMethod: checkMethod,
		Address:     address,
		Port:        port,
		Result:      result,
		Time:        time.Now(),
	}

	// Lock the mutex before updating checkResults
	checkResultsMutex.Lock()
	defer checkResultsMutex.Unlock()

	// Update the checkResult for the client
	checkResults[clientName] = checkResult
	// Print the updated checkResult for the client
	fmt.Printf("Updated check result for client %s: %v\n", clientName, checkResult)

	fmt.Fprintf(w, "Check result submitted successfully")
}

func removeDisconnectedClient(clientName string) {
	disconnectedClientsMutex.Lock()
	defer disconnectedClientsMutex.Unlock()

	for i, name := range disconnectedClients {
		if name == clientName {
			disconnectedClients = append(disconnectedClients[:i], disconnectedClients[i+1:]...)
			break
		}
	}
}

func removeInactiveClients() {
	for {
		time.Sleep(refreshRate)

		if len(disconnectedClients) > 0 {
			time.Sleep(inactivityTimeout)

			disconnectedClientsMutex.Lock()

			for _, name := range disconnectedClients {
				if _, exists := connectedClients[name]; exists {
					delete(connectedClients, name)
					removeDisconnectedClient(name)
				}
			}
			// disconnectedClients = make([]string, 0) // Очистить список неактивных клиентов
			disconnectedClientsMutex.Unlock()
		}
	}
}

func main() {
	// Start the client activity check in the background
	go checkClientActivity()
	go removeInactiveClients()
	go clearTaskPeriodically()

	http.HandleFunc("/", mainPageHandler)
	http.HandleFunc("/connect/clients", clientsPageHandler)
	http.HandleFunc("/get_tasks", getTasksHandler)
	http.HandleFunc("/submit", submitHandler)
	http.HandleFunc("/results_from_clients", resultsFromClientsHandler)
	http.HandleFunc("/update_last_seen", updateLastSeenHandler)
	http.HandleFunc("/clear_all_task", clearAllTasksHandler)
	http.HandleFunc("/clear_all_result", clearAllResultsHandler)

	http.HandleFunc("/results_keeper", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(checkResults)
	})

	// Start the server
	fmt.Println("Server listening on port 8080")
	http.ListenAndServe(":8080", nil)
}
