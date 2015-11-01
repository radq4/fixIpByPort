package main

import (
	"fmt"
	"log"
	"io/ioutil"
	"bufio"
	"os/exec"
	"strings"
	"strconv"	
	"regexp"
	"golang.org/x/crypto/ssh"
)

//Setting
var ipSw = "192.168.4.254"
var ipDhcpRange = "192.168.4"
var ipStartAt = 100
var linkPortSw = [][]int{{25,26,27,28},{49,50,51,52}}
var sw2Mapper = map[int]int{
    1: 101,		2: 102,		3: 103,		4: 104,		5: 49,		6: 50,		7: 51,		8: 52,		9: 53,		10: 54,		11: 55,		12: 56,
    13: 105,	14: 106,	15: 107,	16: 108,	17: 57,		18: 58,		19: 59,		20: 60,		21: 61,		22: 62,		23: 63,		24: 64,
    //Link port
    25: 25,  26: 26, 27: 27,  28: 28,
}
var config = &ssh.ClientConfig{
    User: "cisco",
    Auth: []ssh.AuthMethod{
        ssh.Password("sut@1234"),
    },
    Config: ssh.Config{
			Ciphers: []string{"aes128-cbc"}, // not currently supported
	},
}

var ipAndMacMapping = map[int]string{}

func main() {
	if !checkSwitchSg500() {
		fmt.Println("Error : cannot get SG500")
		return
	}
	//build configuration file
	saveDhcpConf()
}


func checkSwitchSg500() bool {
		client, err := ssh.Dial("tcp", ipSw+":22", config)
		if err != nil {
			log.Fatalf("Failed to dial: " + err.Error())
			return false
		}
		defer client.Close()
		session, err := client.NewSession()
		if err != nil {
			log.Fatalf("unable to create session: %s", err)
			return false
		}
		defer session.Close()
		// Set up terminal modes
		modes := ssh.TerminalModes{
			ssh.TTY_OP_ISPEED: 115200,
			ssh.TTY_OP_OSPEED: 115200,
		}
		// Request pseudo terminal
			if err := session.RequestPty("vt100", 0, 200, modes); err != nil {
				log.Fatalf("request for pseudo terminal failed: %s", err)
			}
			stdin, err := session.StdinPipe()
			if err != nil {
				log.Fatalf("Unable to setup stdin for session: %v\n", err)
			}

			stdout, err := session.StdoutPipe()
			if err != nil {
				log.Fatalf("Unable to setup stdout for session: %v\n", err)
			}
		// Start remote shell
			if err := session.Shell(); err != nil {
				log.Fatalf("failed to start shell: %s", err)
			}
			stdin.Write([]byte("terminal datadump\n"))
			stdin.Write([]byte("show mac address-table\n"))
			scanner := bufio.NewScanner(stdout)
			printMacTable := false
			regMacAddr := regexp.MustCompile(`([0-9a-f]{2}[:-]){5}([0-9a-f]{2})\s+[a-z]{2}\d+/\d+/\d+`)

			for scanner.Scan() {
				s := scanner.Text()
				findMac := regMacAddr.FindString(s)
				if len(findMac) > 0 {
					lineMacIpSplit := strings.Fields(findMac)
					getMac := lineMacIpSplit[0]
					intId := strings.Split(lineMacIpSplit[1],"/")
					getIp, err := strconv.Atoi(intId[2]); if err != nil {
						return false
					}
					intGetSw, err := strconv.Atoi(strings.Replace(intId[0],"gi","",-1)); if err != nil {
						return false
					}
					swLevel := intGetSw-1	
					if(swLevel == 0){
						getIp = sw2Mapper[getIp]	
					}
					if !contains(linkPortSw[swLevel],getIp) {
						ipAndMacMapping[getIp+ipStartAt] = getMac
					}
				//lineBuffer = append(lineBuffer,s)
				}else if (strings.HasPrefix(s,"  Vlan        Mac Address         Port       Type    ")){
					printMacTable = true
				}
				if (len([]byte(s)) == 0 && printMacTable) {
					stdin.Write([]byte("exit\n"))
					client.Close()
					session.Close()
				}
			}
			if err := scanner.Err(); err != nil {
				log.Fatalf("reading standard input: %s", err)
			}

	return true
}

func contains(slice []int, element int) bool {
    for _, item := range slice {
        if item == element {
            return true
        }
    }
    return false
}

func saveDhcpConf(){
	var header = `
############ SETTING ############
######### set dhcp range ########

default-lease-time 600;
max-lease-time 7200;
subnet 192.168.4.0 netmask 255.255.255.0 {
	range 192.168.4.100 192.168.4.200;
	option subnet-mask 255.255.255.0;
	option broadcast-address 192.168.4.255;
	option routers 192.168.4.9;
	option domain-name-servers 8.8.8.8, 8.8.4.4;
	option domain-name "aiyara.lab.sut.ac.th";
} 

######### reserv ip  ########`

var body = ""
for ip,mac := range ipAndMacMapping {
		body = fmt.Sprintf("%s\nhost port-%d {hardware ethernet %s; fixed-address %s.%d; }", body, ip, mac, ipDhcpRange, ip)
}

err := ioutil.WriteFile("/etc/dhcp/dhcpd.conf", []byte(header+body), 0644)
if err != nil {
	log.Fatalf("error writefile: %s",err)
}
cmd := exec.Command("service", "isc-dhcp-server", "restart") 
err = cmd.Run()
if err != nil {
	panic(err)
	log.Fatalf("Cannot restart service %s",err)
}
//fmt.Println(header + body)
}