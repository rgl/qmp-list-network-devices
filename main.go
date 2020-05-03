package main

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/digitalocean/go-qemu/qmp"
	"github.com/mitchellh/go-homedir"
)

type qomListRequest struct {
	Execute   string                  `json:"execute"`
	Arguments qomListRequestArguments `json:"arguments"`
}

type qomListRequestArguments struct {
	Path string `json:"path"`
}

type qomListResponse struct {
	Return []qomListReturn `json:"return"`
}

type qomListReturn struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

func qmpQomList(qmpMonitor *qmp.SocketMonitor, path string) ([]qomListReturn, error) {
	request, _ := json.Marshal(qomListRequest{
		Execute: "qom-list",
		Arguments: qomListRequestArguments{
			Path: path,
		},
	})
	result, err := qmpMonitor.Run(request)
	if err != nil {
		return nil, err
	}
	var response qomListResponse
	if err := json.Unmarshal(result, &response); err != nil {
		return nil, err
	}
	return response.Return, nil
}

type qomGetRequest struct {
	Execute   string                 `json:"execute"`
	Arguments qomGetRequestArguments `json:"arguments"`
}

type qomGetRequestArguments struct {
	Path     string `json:"path"`
	Property string `json:"property"`
}

type qomGetResponse struct {
	Return string `json:"return"`
}

func qmpQomGet(qmpMonitor *qmp.SocketMonitor, path string, property string) (string, error) {
	request, _ := json.Marshal(qomGetRequest{
		Execute: "qom-get",
		Arguments: qomGetRequestArguments{
			Path:     path,
			Property: property,
		},
	})
	result, err := qmpMonitor.Run(request)
	if err != nil {
		return "", err
	}
	var response qomGetResponse
	if err := json.Unmarshal(result, &response); err != nil {
		return "", err
	}
	return response.Return, nil
}

type netDevice struct {
	Path       string
	Name       string
	Type       string
	MacAddress string
}

func getNetDevices(qmpMonitor *qmp.SocketMonitor) ([]netDevice, error) {
	devices := []netDevice{}
	for _, parentPath := range []string{"/machine/peripheral", "/machine/peripheral-anon"} {
		listResponse, err := qmpQomList(qmpMonitor, parentPath)
		if err != nil {
			return nil, fmt.Errorf("failed to get qmp qom list %v: %w", parentPath, err)
		}
		for _, p := range listResponse {
			if strings.HasPrefix(p.Type, "child<") {
				path := fmt.Sprintf("%s/%s", parentPath, p.Name)
				r, err := qmpQomList(qmpMonitor, path)
				if err != nil {
					return nil, fmt.Errorf("failed to get qmp qom list %v: %w", path, err)
				}
				isNetdev := false
				for _, d := range r {
					if d.Name == "netdev" {
						isNetdev = true
						break
					}
				}
				if isNetdev {
					device := netDevice{
						Path: path,
					}
					for _, d := range r {
						if d.Name != "type" && d.Name != "netdev" && d.Name != "mac" {
							continue
						}
						value, err := qmpQomGet(qmpMonitor, path, d.Name)
						if err != nil {
							return nil, fmt.Errorf("failed to get qmp qom property %v %v: %w", path, d.Name, err)
						}
						switch d.Name {
						case "type":
							device.Type = value
						case "netdev":
							device.Name = value
						case "mac":
							device.MacAddress = value
						}
					}
					devices = append(devices, device)
				}
			}
		}
	}
	return devices, nil
}

func getIPAddress(device string, macAddress string) (string, error) {
	// this parses /proc/net/arp to retrive the given device IP address.
	//
	// /proc/net/arp is normally someting alike:
	//
	// 		IP address       HW type     Flags       HW address            Mask     Device
	// 		192.168.121.111  0x1         0x2         52:54:00:12:34:56     *        virbr0
	//

	const (
		IPAddressIndex int = iota
		HWTypeIndex
		FlagsIndex
		HWAddressIndex
		MaskIndex
		DeviceIndex
	)

	// see ARP flags at https://github.com/torvalds/linux/blob/v5.4/include/uapi/linux/if_arp.h#L132
	const (
		AtfCom int = 0x02 // ATF_COM (complete)
	)

	f, err := os.Open("/proc/net/arp")
	if err != nil {
		return "", fmt.Errorf("failed to open /proc/net/arp: %w", err)
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	s.Scan()

	for s.Scan() {
		fields := strings.Fields(s.Text())

		if device != "" && fields[DeviceIndex] != device {
			continue
		}

		if fields[HWAddressIndex] != macAddress {
			continue
		}

		flags, err := strconv.ParseInt(fields[FlagsIndex], 0, 32)
		if err != nil {
			return "", fmt.Errorf("failed to parse /proc/net/arp flags field %s: %w", fields[FlagsIndex], err)
		}

		if int(flags)&AtfCom == AtfCom {
			return fields[IPAddressIndex], nil
		}
	}

	return "", fmt.Errorf("could not find %s", macAddress)
}

func main() {
	log.SetFlags(0)

	if len(os.Args) < 2 {
		log.Fatalf("Usage %s <QMP-SOCKET-ADDRESS>\n", os.Args[0])
	}

	qmpPath, err := homedir.Expand(os.Args[1])
	if err != nil {
		log.Fatalf("ERROR Failed to expand homedir %s: %v", os.Args[1], err)
	}

	qmpMonitor, err := qmp.NewSocketMonitor("unix", qmpPath, 2*time.Second)
	if err != nil {
		log.Fatalf("ERROR Cannot open the QMP monitor at %s: %v", qmpPath, err)
	}

	err = qmpMonitor.Connect()
	if err != nil {
		log.Fatalf("ERROR Cannot connect to QMP monitor at %s: %v", qmpPath, err)
	}
	defer qmpMonitor.Disconnect()

	devices, err := getNetDevices(qmpMonitor)
	if err != nil {
		log.Fatalf("ERROR Cannot retrieve mac addresses from QMP monitor at %s: %v", qmpPath, err)
	}

	w := csv.NewWriter(os.Stdout)
	w.Write([]string{"Name", "Type", "MacAddress", "IpAddress", "Path"})
	for _, d := range devices {
		ipAddress, _ := getIPAddress("", d.MacAddress)
		w.Write([]string{d.Name, d.Type, d.MacAddress, ipAddress, d.Path})
	}
	w.Flush()
}
