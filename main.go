// main.go
package main

import (
    "fmt"
    "os"
    "strings"
    "strconv"
)

// Global constants
const (
    ProgName         = "awsinfo"
    ProgVer          = "2.0.11"
    DNSDataFile      = "dns.json"
    ZoneDataFile     = "zone.json"
    ELBDatafile      = "elb.json"
    InstanceDataFile = "inst.json"
    StackDataFile    = "stack.json"
)

// Global variables
var (
    progConfDir     = ""    // This gets set to $HOME/.${ProgName} in ProcessConfigFile()
    AWSRegion       = ""
    AWSAccountId    = ""
    AWSAccountAlias = ""
    // Below hard-coded values can be overriden via $HOME/.awi/config
    S3Bucket           = "awsinfo"
    S3URLBase          = "https://s3.amazonaws.com/awsinfo"
    APISecondsDelay    = 1
    R53APISecondsDelay = 180
)


func main() {
    ProcessConfigFile()

    // Allow only 1 or 2 arguments; an option with an optional filter
    argCount := len(os.Args[1:])
    option, filter := "", ""
    if argCount == 1 {
        option = os.Args[1]
    } else if argCount == 2 {
        option = os.Args[1]
        // From hereon all filtering comparisons are done in lowercase
        filter = strings.ToLower(os.Args[2])
    } else {
        PrintUsage(option)
    }

    // Process given option with optional filter
    if option == "-u" {
        var minutesAgo int
        var targetZones []string
        // See if optional filter was provided:
        // MIN minutes = update if any records have changed MIN ago
        // ZONE zone = update only the mentioned zones, if owned by user
        if filter != "" {
            if minInt, err := strconv.Atoi(filter); err == nil {
                if minInt < 1 || minInt > 10080 {
                    Die(1, "Error. MIN minutes (" + filter + ") must be between 1 and 10080 (7 days).")
                }
                minutesAgo = minInt
            } else {
                targetZones = strings.Split(filter, ",")
            }
        }
        // Setup AWS access and update all stores
        SetupAWSAccess()
        UpdateLocalInstanceStoreFromAWS(minutesAgo)
        UpdateLocalZoneStoreFromAWS(minutesAgo)
        UpdateLocalELBStoreFromAWS(minutesAgo)
        UpdateLocalStackStoreFromAWS(minutesAgo)
        UpdateLocalDNSStoreFromAWS(targetZones, minutesAgo)
    } else if option == "-3" || option == "-3f" {
        SetupAWSAccess()
        CopyLocalStoresToS3Bucket(option)
    } else if option == "-x" {
        DeleteLocalStoresFiles("verbose")
    } else if option == "-y" {
        CreateSkeltonConfigFile()
    } else if option == "-z" {
        ListZones(filter)
    } else if option == "-d" || option == "-dv"  {
        ListDNS(filter, option)
    } else if option == "-e" {
        ListELBRecords(filter)
    } else if option == "-es" {
        ListELBCerts(filter)
    } else if option == "-eh" {
        ListELBHealthChecks(filter)    
    } else if option == "-i" || option == "-iv" {
        ListInstances(filter, option)
    } else if option == "-s" || option == "-sv" {
        ListStacks(filter, option)
    } else if filter != "" || option == "-h" {
        PrintUsage(option)
    } else {
        BreakdownDNS(option)   // Default to this as the only remaining option
    }
    Die(0,"")
}


func PrintUsage(option string) {
    fmt.Printf("AWS CLI Information Utility %s\n", ProgVer)
    fmt.Printf("%s DNSRECORD        Print IPs/ELB/instances breakdown for given DNSRECORD\n", ProgName)
    fmt.Printf("        -e  [STRING]     List ELBs, filter with optional STRING\n")
    fmt.Printf("        -d  [STRING]     List DNS records, filter with optional STRING\n")
    fmt.Printf("        -i  [STRING]     List EC2 instances, filter with optional STRING\n")
    fmt.Printf("        -s  [STRING]     List CloudFormation stacks, filter with optional STRING\n")
    fmt.Printf("        -z  [STRING]     List DNS zones, filter with optional STRING\n")
    fmt.Printf("        -h               Show extended options\n")
    if option == "-h" {
        fmt.Printf("        -3               Copy local stores to S3 bucket defined in ~/.%s/config\n", ProgName)
        fmt.Printf("        -3f              Ignore file time stamps and force above copying\n")
        fmt.Printf("        -eh [STRING]     List ELB health-checks, filter with optional STRING\n")
        fmt.Printf("        -es [STRING]     List ELB SSL certs, filter with optional STRING\n")
        fmt.Printf("        -dv [STRING]     List DNS records, more verbosely\n")
        fmt.Printf("        -iv [STRING]     List EC2 instances, more verbosely\n")
        fmt.Printf("        -sv [STRING]     List CloudFormation stacks, more verbosely\n")
        fmt.Printf("        -u  [MIN|ZONES]  Update local stores, and only DNS records changed in last MIN minutes,\n")
        fmt.Printf("                         or from zones in ZONES string, e.g., 'mysite.com,a.mydns.com,site.io'\n")
        fmt.Printf("        -x               Delete local store, to start afresh\n")
        fmt.Printf("        -y               Create skeleton ~/.%s/config file\n", ProgName)
    }
    Die(0,"")
}
