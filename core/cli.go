package core

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"
)

func StartCLI(n *Node) {
	reader := bufio.NewReader(os.Stdin)

	for {
		printMainMenu()

		fmt.Print("Enter command: ")
		command, err := reader.ReadString('\n')
		if err != nil {
			log.Printf("Failed to read command: %v\n", err)
			continue
		}

		command = strings.TrimSpace(command)

		switch command {
		case "h":
			printHelp()
		case "c":
			printCommandList()
			fmt.Print("Choose a command: ")
			subCommand, err := reader.ReadString('\n')
			if err != nil {
				log.Printf("Failed to read sub-command: %v\n", err)
				continue
			}
			subCommand = strings.TrimSpace(subCommand)
			handleSubCommand(subCommand, reader, n)
		case "r":
			continue
		default:
			fmt.Println("Unknown command. Type 'h' for help.")
		}
	}
}

func printMainMenu() {
	fmt.Println("Main Menu:")
	fmt.Println("h: Help")
	fmt.Println("c: Command List")
	fmt.Println("r: Return to main menu")
}

func printHelp() {
	fmt.Println("Help - Display information about available commands")
	fmt.Println("c: Command List - Display a list of available commands")
	fmt.Println("r: Return to main menu - Return to the main menu")
}

func printCommandList() {
	fmt.Println("Command List:")
	fmt.Println("1: Send Message")
	fmt.Println("2: List Users")
	fmt.Println("3: Node Information")
	fmt.Println("4: Check Topology")
	fmt.Println("5: Exit")
	fmt.Println("6: Kill")
}

func handleSubCommand(subCommand string, reader *bufio.Reader, n *Node) {
	switch subCommand {
	case "1":
		fmt.Println("Send Message - Selecting recipient and sending message:")

		users, err := n.GetUsers()
		if err != nil {
			logger.App.Println("Failed to get users:", err)
			return
		}

		fmt.Println("Select recipient:")
		for index, user := range users {
			fmt.Printf("%d: %d\n", index, user)
		}

		fmt.Print("Enter recipient index: ")
		var recipientIndex int
		_, err = fmt.Scan(&recipientIndex)
		if err != nil || recipientIndex < 0 || recipientIndex >= len(users) {
			fmt.Println("Invalid recipient index. Please choose a valid index.")
			return
		}

		var message string
		fmt.Print("Enter message: ")
		message, err = reader.ReadString('\n')
		if err != nil {
			logger.App.Println("Failed to read message:", err)
			return
		}

		recipientID := users[recipientIndex]
		err = n.SendMessage(recipientID, strings.TrimRight(message, "\n"))
		if err != nil {
			logger.App.Println("Failed to send message:", err)
		}
	case "2":
		fmt.Println("List Users - Displaying list of users:")
		users, err := n.GetUsers()
		if err != nil {
			logger.App.Println("Failed to get users:", err)
			return
		}

		for index, user := range users {
			fmt.Printf("%d: %d\n", index, user)
		}
	case "3":
		fmt.Println("Node Information - Displaying information about the current node:")
		fmt.Println(n)
	case "4":
		fmt.Println("Check Topology - Check current ring topology:")
		res := n.CheckTopology()
		if res {
			fmt.Println("Topology is correct")
		} else {
			fmt.Println("Topology is incorrect")
		}
	case "5":
		fmt.Println("Exit - Disconnecting from the network")
		_ = n.Disconnect()
		os.Exit(0)
	case "6":
		fmt.Println("Kill - Simulated application failure (Panic)")
		panic("Simulated application failure")
	default:
		fmt.Println("Invalid sub-command. Please choose a valid sub-command.")
	}
}
