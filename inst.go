// inst.go
package main

import (
    "fmt"
    "time"
    "strings"
    "encoding/json"
    "github.com/aws/aws-sdk-go/aws"
    "github.com/aws/aws-sdk-go/aws/session"
    "github.com/aws/aws-sdk-go/service/ec2"
    "github.com/aws/aws-sdk-go/aws/awsutil"
)


// Extend AWS ec2.Instance type to include these additional fields
type InstanceType struct {
    AccountAlias  *string
    AccountId     *string
    *ec2.Instance
}

// Return string representation of this type
func (s InstanceType) String() string {
    return awsutil.Prettify(s)
}

// Set AccountAlias field's value
func (s *InstanceType) SetAccountAlias(v string) *InstanceType {
    s.AccountAlias = &v
    return s
}

// Set AccountId field's value
func (s *InstanceType) SetAccountId(v string) *InstanceType {
    s.AccountId = &v
    return s
}

// Display all EC2 instances with applied filter
func ListInstances(filter string, option string) {
    instList, err := GetInstanceList()
    if err != nil {
        Die(1, err.Error())
    }
    for _, inst := range instList {
        // Using single letters for better readability
        a, b, c, d, e, f, g, h, k, l, m, n, o, p := GetInstanceDetails(&inst)
        // Apply filter string on all attributes
        if filter == "" || strContains(a, filter) || strContains(b, filter) ||
           strContains(c, filter) || strContains(d, filter) || strContains(e, filter) ||
           strContains(f, filter) || strContains(g, filter) || strContains(h, filter) ||
           strContains(k, filter) || strContains(l, filter) || strContains(m, filter) ||
           strContains(n, filter) || strContains(o, filter) || strContains(p, filter) {

            //  Replace spaces with period and shorten names
            if len(a) > 38 { a = a[:38] }
            a = strings.Replace(a, " ", ".", -1)

            if option == "-iv" {
                fmt.Printf("%-38s  %-20s  %-12s  %-10s  %-16s  %-18s  " + 
                           "%-18s  %-12s  %-6s  %-12s  %-16s  %-14s  %-14s  %-70s\n",
                           a, b, c, d, e, f, g, h, k, l, m, n, o, p)
            } else {
                fmt.Printf("%-38s  %-20s  %-12s  %-10s  %-16s  %-18s  %-18s\n", a, b, c, d, e, f, g)
            }
        }
    }
}


// Return newest instance list between local and remote store
func GetInstanceList() (list []InstanceType, err error) {
    localFileTimestamp := GetLocalFileTime(InstanceDataFile)
    remoteFileTimestamp := GetRemoteFileTime(InstanceDataFile)

    // Use remote S3 file if it's newer
    if remoteFileTimestamp.After(localFileTimestamp) {
        tmplist, err := GetListFromRemote(InstanceDataFile)
        list = tmplist.([]InstanceType)    // Assert our Instance type
        if err == nil {
            WriteList(list, InstanceDataFile)   // Update local with this newer set
            return list, nil
        }
        // Return what must be an empty list with the error code
        return list, err
    }

    // Else, just return local file content with error code
    tmplist, err := GetListFromLocal(InstanceDataFile)
    list = tmplist.([]InstanceType)
    return list, err
}


// Return important attributes of given object
func GetInstanceDetails(inst *InstanceType) (Name string,             // a
                                            InstanceId string,        // b  
                                            InstanceType string,      // c
                                            State string,             // d
                                            IPAddr string,            // e
                                            AccountAlias string,      // f
                                            LaunchTime string,        // g
                                            Environment string,       // h
                                            BillingBrandCode string,  // k
                                            AvailZone string,         // l
                                            SubnetId string,          // m
                                            ImageId string,           // n
                                            KeyName string,           // o
                                            iamProf string) {         // p
    Name, InstanceId, InstanceType = "-", "-", "-"
    State, IPAddr, AccountAlias = "Unknown", "-", "-"
    LaunchTime, Environment, BillingBrandCode = "-", "-", "-"
    AvailZone, SubnetId, ImageId, KeyName, iamProf = "-", "-", "-", "-", "-"

    // Note how we defensibly ensure pointer values aren't 'nil'

    // Get tag attributes
    for _, Tag := range inst.Tags {
        if Tag.Key != nil && Tag.Value != nil {
            if *Tag.Key == "Name" { Name = *Tag.Value }
            if *Tag.Key == "Environment" { Environment = *Tag.Value }
            if *Tag.Key == "BillingBrandCode" { BillingBrandCode = *Tag.Value }
        }
    }
    if inst.InstanceId != nil { InstanceId = *inst.InstanceId }
    if inst.InstanceType != nil { InstanceType = *inst.InstanceType }
    if inst.State.Name != nil { State = *inst.State.Name }
    if inst.PrivateIpAddress != nil { IPAddr = *inst.PrivateIpAddress }
    if inst.AccountAlias != nil { AccountAlias = *inst.AccountAlias }
    if inst.LaunchTime != nil {
        lt := *inst.LaunchTime
        LaunchTime = lt.Format("2006-01-02 15:04")
    }
    if inst.Placement != nil {
        if inst.Placement.AvailabilityZone != nil {
            AvailZone = *inst.Placement.AvailabilityZone
        }
    }
    if inst.SubnetId != nil { SubnetId = *inst.SubnetId }
    if inst.ImageId != nil { ImageId = *inst.ImageId }
    if inst.KeyName != nil { KeyName = *inst.KeyName }
    if inst.IamInstanceProfile != nil {
        if inst.IamInstanceProfile.Arn != nil {
            iamProf = *inst.IamInstanceProfile.Arn
        }
    }
    return
}


// Update local instance store from current AWS account
func UpdateLocalInstanceStoreFromAWS(minutesAgo int) {
    // Do full update if minutesAgo is zero (meaning it wasn't specified)
    if minutesAgo == 0 {
        fmt.Printf("Updating local EC2 instance store.\n")
    } else {
        updatedInstCount := len(GetCloudTrailEvents("ec2", minutesAgo))
        if updatedInstCount < 1 {
            // Skip update if no EC2 events within ast minutesAgo
            fmt.Printf("Skipping local EC2 instance store update (no mods within %d minutes)\n",
                minutesAgo)
            return
        }
        fmt.Printf("Updating local EC2 instance store (%d modified within %d minutes)\n",
            updatedInstCount, minutesAgo)
    }

    // Create a new list from existing store, without the ones for the current AWS account
    var list []InstanceType
    instList, _ := GetInstanceList()
    for _, inst := range instList {
        if *inst.AccountId != AWSAccountId {
            list = append(list, inst)
        }
    }

    // Now get all records for this account, and add them to this new list
    for _, inst := range GetInstanceListFromAWS() {
        list = append(list, inst)
    }

    // Make this the new local list
    WriteList(list, InstanceDataFile)
    return
}


// Return all instance objects in current AWS account
func GetInstanceListFromAWS() (list []InstanceType) {
    SetAWSRegion()
    sess := session.Must(session.NewSession())
    svc := ec2.New(sess, aws.NewConfig().WithRegion(AWSRegion))

    params := &ec2.DescribeInstancesInput{
        MaxResults: aws.Int64(500),   // Max is 1000, but we'll get in 500 size sets
    }

    // Loop requests in case there're more than PageSize records or we're being throttled
    errcount := 0
    for {
        // Get batch of records
        resp, err := svc.DescribeInstances(params)
        if err != nil {
            // Sleep for a moment if AWS is throttling us
            if BeingThrottled(err) {
                fmt.Printf("  AWS throttling. Sleeping %d seconds...\n", APISecondsDelay)
                time.Sleep(time.Duration(APISecondsDelay) * time.Second)
                continue
            }
            // Allow for 3 other unknown API call errors before panicking
            if errcount < 3 {
                errcount++
                continue
            }
            panic(err.Error())   // Abort on any other error
        }

        // Ensure valid data came back
        if resp.Reservations != nil {
            // Instance records are under the Reservations list
            for i, _ := range resp.Reservations {
                if resp.Reservations[i].Instances != nil {
                    // Decode this batch into our extended type
                    // First convert it to raw []byte
                    jsonData, err := json.Marshal(resp.Reservations[i].Instances)
                    if err != nil {
                        panic(err.Error())
                    }
                    // Now read it into extended type list
                    var instList []InstanceType
                    err = json.Unmarshal(jsonData, &instList)
                    if err != nil {
                        panic(err.Error())
                    }
                    // Add this batch to our list
                    for _, inst := range instList {
                        // Add our additional fields
                        inst = *inst.SetAccountAlias(AWSAccountAlias)
                        inst = *inst.SetAccountId(AWSAccountId)
                        list = append(list, inst)
                    }
                }
            }
        }

        // Exit loop if no more records, else setup next batch request
        if resp.NextToken == nil {
            break
        } else {
            params.NextToken = resp.NextToken
        }
    }
    return list
}
