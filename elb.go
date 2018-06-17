// elb.go
package main

import (
    "fmt"
    "time"
    "strings"
    "strconv"
    "errors"
    "encoding/json"
    "github.com/aws/aws-sdk-go/aws"
    "github.com/aws/aws-sdk-go/aws/session"
    "github.com/aws/aws-sdk-go/service/elb"
    "github.com/aws/aws-sdk-go/aws/awsutil"
)

// Extend AWS elb.LoadBalancerDescription type to include these additional fields
type LoadBalancerDescriptionType struct {
    AccountAlias  *string
    AccountId     *string
    *elb.LoadBalancerDescription
}

// Return string representation of this type
func (s LoadBalancerDescriptionType) String() string {
    return awsutil.Prettify(s)
}

// Set AccountAlias field's value
func (s *LoadBalancerDescriptionType) SetAccountAlias(v string) *LoadBalancerDescriptionType {
    s.AccountAlias = &v
    return s
}

// Set AccountId field's value
func (s *LoadBalancerDescriptionType) SetAccountId(v string) *LoadBalancerDescriptionType {
    s.AccountId = &v
    return s
}

// Display all ELB records with applied filter
func ListELBRecords(filter string) {
    elbList, err := GetELBList()
    if err != nil {
        Die(1, err.Error())
    }
    for _, elbRec := range elbList {
        elbName, elbDNSName, instCount, instIds := GetDetailsOfELB(elbRec)
        instances := ""   // Build instances strings
        for i := 0 ; i < instCount ; i++ {
            instances = instances + instIds[i] + " "
        }
        instances = strings.TrimSpace(instances)
        if filter == "" || strContains(elbName, filter) ||
                           strContains(elbDNSName, filter) ||
                           strContains(instances, filter) {
            fmt.Printf("%-36s  %-80s  %4d  %s\n", elbName, elbDNSName, instCount, instances)
        }
    }
    return
}


// Return elb records list in local or remote store
func GetELBList() (list []LoadBalancerDescriptionType, err error) {
    localFileTimestamp := GetLocalFileTime(ELBDatafile)
    remoteFileTimestamp := GetRemoteFileTime(ELBDatafile)

    // Use remote S3 file if it's newer
    if remoteFileTimestamp.After(localFileTimestamp) {
        tmplist, err := GetListFromRemote(ELBDatafile)
        list = tmplist.([]LoadBalancerDescriptionType)  // Assert our ELB type
        if err == nil {
            WriteList(list, ELBDatafile)          // Update local with this newer set
            return list, nil
        }        
        // Return what must be an empty list with the error code
        return list, err
    }

    // Else, just return local file content with error code
    tmplist, err := GetListFromLocal(ELBDatafile)
    list = tmplist.([]LoadBalancerDescriptionType)
    return list, err
}


// Display all ELB health checks with applied filter
func ListELBHealthChecks(filter string) {
    elbList, err := GetELBList()
    if err != nil {
        Die(1, err.Error())
    }
    for _, elbRec := range elbList {
        dns, healthy, unhealthy, interval, timeout, target := "-", "-", "-", "-", "-", "-"
        if elbRec.HealthCheck != nil { 
            if elbRec.DNSName != nil { dns = *elbRec.DNSName }
            if elbRec.DNSName != nil { dns = *elbRec.DNSName }
            if elbRec.HealthCheck.HealthyThreshold != nil {
                healthy = strconv.FormatInt(*elbRec.HealthCheck.HealthyThreshold, 10)
            }
            if elbRec.HealthCheck.UnhealthyThreshold != nil {
                unhealthy = strconv.FormatInt(*elbRec.HealthCheck.UnhealthyThreshold, 10)
            }
            if elbRec.HealthCheck.Interval != nil {
                interval = strconv.FormatInt(*elbRec.HealthCheck.Interval, 10)
            }
            if elbRec.HealthCheck.Timeout != nil {
                timeout = strconv.FormatInt(*elbRec.HealthCheck.Timeout, 10)
            }
            if elbRec.HealthCheck.Target != nil {
                target = *elbRec.HealthCheck.Target
            }
            // Print only if qualified by filter
            if filter == "" || strContains(dns, filter) || strContains(target, filter) {
                fmt.Printf("%-80s  %4s  %4s  %4s  %4s  %s\n",
                    dns, healthy, unhealthy, interval, timeout, target)
            }
        }
    }
}


// Display all ELB certs with applied filter
func ListELBCerts(filter string) {
    elbList, err := GetELBList()
    if err != nil {
        Die(1, err.Error())
    }
    for _, elbRec := range elbList {
        dns, cert := "-", "-"
        if elbRec.ListenerDescriptions != nil && len(elbRec.ListenerDescriptions) > 0 {
            for _, l := range elbRec.ListenerDescriptions {
                if l.Listener != nil && l.Listener.Protocol != nil &&
                   *l.Listener.Protocol == "HTTPS" {
                    if l.Listener.SSLCertificateId != nil {
                        cert = *l.Listener.SSLCertificateId
                    }
                }
            }
        }
        if elbRec.DNSName != nil {
            dns = *elbRec.DNSName
        }
        // Print only if qualified by filter
        if filter == "" || strContains(dns, filter) || strContains(cert, filter) {
            fmt.Printf("%-80s  %s\n", dns, cert)
        }
    }
}


// Return important attributes of given object
func GetDetailsOfELB(elbRec LoadBalancerDescriptionType) (elbName, elbDNSName string,
                                                          instCount int, instIds []string) {
    elbName, elbDNSName, instCount, instIds = "-", "-", 0, nil

    if elbRec.LoadBalancerName != nil { elbName = *elbRec.LoadBalancerName }
    if elbRec.DNSName != nil { elbDNSName = *elbRec.DNSName }
    if elbRec.Instances != nil { instCount = len(elbRec.Instances) }

    // Build list of instances Ids
    if instCount > 0 {
        for i := 0 ; i < instCount ; i++ {
            instIds = append(instIds, *elbRec.Instances[i].InstanceId)
        }
    }
    return
}


// Breakdown given ELB DNS name into its ELB/instances backend components
func BreakdownELB(elbDNSName string) {
    // Dont do anything if an ELB with that DNS name doesnt exist
    elb, err := GetELBFromLocal(elbDNSName)
    if err != nil {
        return
    }

    fmt.Println(elbDNSName)

    // Print listener ports and target instance ports
    listenerCount := 0
    if elb.ListenerDescriptions != nil {
        listenerCount = len(elb.ListenerDescriptions)
        if listenerCount > 0 {
            for x := 0 ; x < listenerCount ; x++ {
                listDesc := elb.ListenerDescriptions[x]
                if listDesc != nil {
                    lstner := listDesc.Listener
                    if lstner != nil {
                        lbPort := ""
                        if lstner.LoadBalancerPort != nil {
                            lbPort = strconv.FormatInt(*lstner.LoadBalancerPort, 10)
                        }
                        instPort := ""
                        if lstner.InstancePort != nil {
                            instPort = strconv.FormatInt(*lstner.InstancePort, 10)
                        }
                        fmt.Printf("  %-5s -> %5s\n", lbPort, instPort)
                    }
                }
            }
        }
    }

    if listenerCount == 0 {
        fmt.Println("  No listeners defined")
    }

    // Print instances
    if elb.Instances != nil {
        instCount := len(elb.Instances)
        if instCount > 0 {
            masterInstList, err := GetInstanceList()   // For efficiency, get instance store list here
            if err != nil {
                panic(err.Error())
            }
            for x := 0 ; x < instCount ; x++ {
                inst := elb.Instances[x]
                if inst != nil {
                    if inst.InstanceId != nil {
                        notfound := true
                        for _, i := range masterInstList{
                            if i.InstanceId != nil &&
                               strings.EqualFold(*i.InstanceId, *inst.InstanceId) {
                                a, b, c, d, e, f, _, _, _, _, _, _, _, _ := GetInstanceDetails(&i)
                                // a = Name     b = InstanceId    c = InstanceType
                                // d = State    e = IPAddr        f = AccountAlias
                                fmt.Printf("    %-38s  %-20s  %-12s  %-10s  %-16s  %-16s\n",
                                           a, b, c, d, e, f)
                                notfound = false
                                break
                            }
                        }
                        if notfound {
                            fmt.Printf("    %s not found in instance store\n", *inst.InstanceId)
                        }
                    }
                }
            }
        }
    }
    return
}


// Return specific ELB record name, if it exists in local store
func GetELBFromLocal(elbDNSName string) (LoadBalancerDescriptionType, error) {
    empty := LoadBalancerDescriptionType{}         // Empty record
    elbList, err := GetELBList()
    if err != nil {
        panic(err.Error())
    }
    for _, elb := range elbList {
        if elb.DNSName != nil {
            if strings.EqualFold(NormalDNSName(elbDNSName), NormalDNSName(*elb.DNSName)) {
                return elb, nil
            }            
        }
    }
    return empty, errors.New("Record not found.")  // Return empty record
}


// Update local copy of ELB records from current AWS account
func UpdateLocalELBStoreFromAWS(minutesAgo int) {
    // Do full update if minutesAgo is zero (meaning it wasn't specified)
    if minutesAgo == 0 {
        fmt.Printf("Updating local ELB store.\n")
    } else {
        updatedELBCount := len(GetCloudTrailEvents("elasticloadbalancing", minutesAgo)) 
        if updatedELBCount < 1 {
            // Skip ELB update if no ELB events within last minutesAgo
            fmt.Printf("Skipping local ELB store update (no mods within %d minutes)\n",
                minutesAgo)
            return
        }
        fmt.Printf("Updating local ELB store (%d modified within %d minutes)\n",
            updatedELBCount, minutesAgo)
    }

    // Create a new list from existing store, without the ones for the current AWS account
    var list []LoadBalancerDescriptionType
    elbList, _ := GetELBList()
    for _, elb := range elbList {
        if *elb.AccountId != AWSAccountId {
            list = append(list, elb)
        }
    }

    // Now get all records for this account, and add them to this new list
    for _, elb := range GetELBListFromAWS() {
        list = append(list, elb)
    }

    // Make this the new local list
    WriteList(list, ELBDatafile)
    return
}


// Return all ELB objects in current AWS account
func GetELBListFromAWS() (list []LoadBalancerDescriptionType) {
    SetAWSRegion()
    sess := session.Must(session.NewSession())
    svc := elb.New(sess, aws.NewConfig().WithRegion(AWSRegion))

    params := &elb.DescribeLoadBalancersInput{
        PageSize: aws.Int64(400),  // 400 is AWS max request limit
    }
    // Loop requests in case there're more than PageSize records or we're being throttled
    errcount := 0
    for {
        // Get batch of records
        resp, err := svc.DescribeLoadBalancers(params)
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
        if resp.LoadBalancerDescriptions != nil {
            // Decode this batch into our extended type
            // First convert it to raw []byte
            jsonData, err := json.Marshal(resp.LoadBalancerDescriptions)
            if err != nil {
                panic(err.Error())
            }
            // Now read it into extended type list
            var elbList []LoadBalancerDescriptionType
            err = json.Unmarshal(jsonData, &elbList)
            if err != nil {
                panic(err.Error())
            }
            // Add this batch to our list
            for _, elb := range elbList {
                // Add our additional fields
                elb = *elb.SetAccountAlias(AWSAccountAlias)
                elb = *elb.SetAccountId(AWSAccountId)
                list = append(list, elb)
            }
        }

        // Exit loop if no more records, else setup next batch request
        if resp.NextMarker == nil {
            break
        } else {
            params.Marker = resp.NextMarker
        }
    }
    return list
}
