// dns.go
package main

import (
    "fmt"
    "net"
    "time"
    "strings"
    "strconv"
    "errors"
    "github.com/aws/aws-sdk-go/aws"
    "github.com/aws/aws-sdk-go/aws/session"
    "github.com/aws/aws-sdk-go/service/route53"
    "github.com/aws/aws-sdk-go/aws/awsutil"
    "encoding/json"
)

// Extend AWS route53.ResourceRecordSet type to include these additional fields
type ResourceRecordSetType struct {
    AccountAlias  *string
    AccountId     *string
    ZoneId        *string
    *route53.ResourceRecordSet
}

// Return string representation of this type
func (s ResourceRecordSetType) String() string {
    return awsutil.Prettify(s)
}

// Set AccountAlias field's value
func (s *ResourceRecordSetType) SetAccountAlias(v string) *ResourceRecordSetType {
    s.AccountAlias = &v
    return s
}

// Set AccountId field's value
func (s *ResourceRecordSetType) SetAccountId(v string) *ResourceRecordSetType {
    s.AccountId = &v
    return s
}

// Set ZoneId field's value
func (s *ResourceRecordSetType) SetZoneId(v string) *ResourceRecordSetType {
    s.ZoneId = &v
    return s
}


// Display all DNS records with applied filter
func ListDNS(filter string) {
    list, err := GetDNSList()
    if err != nil {
        Die(1, err.Error())
    }
    for _, dnsRec := range list {
        dnsName, dnsType, dnsTTL, dnsZoneId, dnsCount, dnsValues := GetDetailsOfDNS(dnsRec)
        // We only care to list CNAME, ALIAS, and A records
        if strings.EqualFold(dnsType, "cname") ||
           strings.EqualFold(dnsType, "alias") ||
           strings.EqualFold(dnsType, "a") { 
            Values := ""
            for i := 0 ; i < dnsCount ; i++ {
                Values = Values + dnsValues[i] + " "
            }
            Values = strings.TrimSpace(Values)
            if filter == "" || strContains(dnsName, filter) || strContains(Values, filter) ||
                               strContains(dnsType, filter) || strContains(dnsTTL, filter) ||
                               strContains(dnsZoneId, filter) {
                // Notice we don't actually display d.ZoneID but do filter by it
                fmt.Printf("%-64s  %-8s  %6s  %-2d  %s\n", dnsName, dnsType, dnsTTL, dnsCount, Values)
            }
        }
    }
    return
}


// Return dns records list in local or remote store
func GetDNSList() (list []ResourceRecordSetType, err error) {
    localFileTimestamp := GetLocalFileTime(DNSDataFile)
    remoteFileTimestamp := GetRemoteFileTime(DNSDataFile)

    // Use remote S3 file if it's newer
    if remoteFileTimestamp.After(localFileTimestamp) {
        tmplist, err := GetListFromRemote(DNSDataFile)
        list = tmplist.([]ResourceRecordSetType)  // Assert our DNS type
        if err == nil {
            WriteList(list, DNSDataFile)          // Update local with this newer set
            return list, nil
        }
        // Return what must be an empty list with the error code
        return list, err
    }

    // Else, just return local file content with error code
    tmplist, err := GetListFromLocal(DNSDataFile)
    list = tmplist.([]ResourceRecordSetType)
    return list, err
}


// Return important attributes of given object
func GetDetailsOfDNS(dnsRec ResourceRecordSetType) (dnsName, dnsType, dnsTTL, dnsZoneId string,
                                                    dnsCount int, dnsValues []string) {
    dnsName, dnsType, dnsTTL, dnsValues, dnsCount = "-", "-", "-", nil, 0

    dnsName = strings.TrimSuffix(*dnsRec.Name, ".")  // Trim superfluous pre/suffixes in Name
    dnsZoneId = *dnsRec.ZoneId

    // Check only for CNAME and A records. Skip all others types
    if *dnsRec.Type == route53.RRTypeCname {        // If Type is CNAME
        dnsType = "CNAME"                           // CNAMEs only have 1 value, index 0
        dnsCount = 1
        dnsValues = append(dnsValues, *dnsRec.ResourceRecords[0].Value)
        dnsValues[0] = strings.TrimSuffix(dnsValues[0], ".")   // Trim superfluous pre/suffixes in CNAME
    } else if *dnsRec.Type == route53.RRTypeA {     // Decipher between regular A and ALIAS record
        if dnsRec.ResourceRecords == nil {          // If nil then it's an ALIAS record 
            dnsType = "ALIAS"
            dnsCount = 1
            dnsValues = append(dnsValues, *dnsRec.AliasTarget.DNSName)
            dnsValues[0] = NormalDNSName(dnsValues[0])
        } else {
            dnsType = "A"                           // It's a regular A record
            dnsCount = len(dnsRec.ResourceRecords)  // Get the number of values (IPs)
            for i := 0 ; i < dnsCount ; i++ {
                dnsValues = append(dnsValues, *dnsRec.ResourceRecords[i].Value)
            }
        }        
    } else {
        return
    }

    if dnsRec.TTL != nil { dnsTTL = strconv.FormatInt(*dnsRec.TTL, 10) } // Convert TTL to string
    return
}


// Breakdown given DNS name into its ELB/instances backend components
func BreakdownDNS(dnsName string) {
    // Skip all CNAMEs until we find the A record at the end
    lastARec := dnsName
    for {
        resp, err := net.LookupCNAME(lastARec)
        if err != nil {
            Die(1, err.Error())          // Abort if record points to nowhere
        }
        respRec := NormalDNSName(resp)   // Normalize DNS name
        if strings.EqualFold(respRec, lastARec) {
            lastARec = respRec           // Exit for-loop if same as last result
            break
        }
        lastARec = respRec               // Try again with this last record
    }

    // If last-A-record is one of our ELB DNS names then do ELB breakdown
    _, err := GetELBFromLocal(lastARec)
    if err == nil {
        BreakdownELB(lastARec)
        return
    }

    // If last-A-record is one our DNS records and points to one of our ELBs (ALIAS record) then do ELB breakdown
    dns, err := GetDNSFromLocal(lastARec)
    if err == nil {
        if dns.AliasTarget != nil {
            elbDNSName := dns.AliasTarget.DNSName
            if elbDNSName != nil {
                BreakdownELB(NormalDNSName(*elbDNSName))
            }
        }
    }
    return
}


// Normalize DNS name by removing superfluous 'dualstack.' and '.' strings
func NormalDNSName(dnsName string) string {
    str := strings.TrimPrefix(dnsName, "dualstack.")
    return strings.TrimSuffix(str, ".")
}


// Return specific DNS record name, if it exists in local store
func GetDNSFromLocal(dnsName string) (dns ResourceRecordSetType, err error) {
    empty := ResourceRecordSetType{}               // Empty record
    list, err := GetDNSList()
    if err != nil {
        return dns, err
    }
    for _, rec := range list {
        if rec.Name != nil {        
            if strings.EqualFold(NormalDNSName(dnsName), NormalDNSName(*rec.Name)) {
                return rec, nil
            }
        }
    }
    return empty, errors.New("Record not found.")  // Return empty record
}


// Update local copy of DNS records from current AWS account
func UpdateLocalDNSStoreFromAWS(targetZones []string, minutesAgo int) {
    // Note that DNS updates _have_ to be done by going thru each DNS zone

    // Start with the most recent zone store set
    currentZoneList, _ := GetZoneList()
    var targetZoneList []HostedZoneType

    // Determine whether we're doing all zones, recently changed ones, or only those specified by user 
    if len(targetZones) == 0 && minutesAgo == 0 {
        // We'll update DNS records in ALL the zones
        fmt.Printf("Updating local DNS store for all domains (can take a long time).\n")
        targetZoneList = currentZoneList
    } else if len(targetZones) == 0 && minutesAgo > 0 {
        // We'll update DNS records ONLY in zones modified within minutesAgo
        updatedZoneIdList := GetUpdatedZoneIdList(minutesAgo)
        updatedZoneIdListCount := len(updatedZoneIdList)
        if updatedZoneIdListCount < 1 {
            fmt.Printf("Skipping local DNS store update (no mods within %d minutes)\n",
                minutesAgo)
            return
        }
        fmt.Printf("Updating local DNS store (%d modified within %d minutes).\n",
            updatedZoneIdListCount, minutesAgo)
        // Build our specific target list
        for _, zone := range currentZoneList {
            // Add zone record only if it's one of the ones recently changed
            if strListContains(updatedZoneIdList, *zone.Id) {
                targetZoneList = append(targetZoneList, zone)
            }
        }
    } else if len(targetZones) > 0 && minutesAgo == 0 {
        // We'll update DNS records ONLY in zones specified in targetZones
        str := strings.Join(targetZones[:],", ")
        fmt.Printf("Updating local DNS store for the following zones: %s\n", str)
        // Build our specific target list
        for _, zone := range currentZoneList {
            // Add zone record only if it's one of the ones specified by user
            zoneName := strings.TrimSuffix(*zone.Name, ".")  // Remove useless dotted suffix
            if strListContains(targetZones, zoneName) {
                targetZoneList = append(targetZoneList, zone)
            }
        }
    }

    // Create a new list from existing store, without the ones in the target zones
    var list []ResourceRecordSetType 
    dnsList, _ := GetDNSList()
    for _, dns := range dnsList  {
        // Add this record to our new list ONLY if it's NOT in one of our target zones
        zone, err := GetZoneByIdFromList(currentZoneList, *dns.ZoneId)
        if err != nil {
            if !zoneListContains(targetZoneList, zone) {
                list = append(list, dns)
            }
        }
    }

    // Now get all records for this account, and add them to this new list
    // Cycle thru each target zone and update their DNS records
    for _, zone := range targetZoneList {
        // We only care to update those records for current AWS account
        if *zone.AccountId != AWSAccountId {
            continue
        }

        // Now get all records for this account/zone, and add them to this new list
        dnsList := GetDNSListByZoneIdFromAWS(*zone.Id)
        // Print some info in the process
        zoneName := strings.TrimSuffix(*zone.Name, ".")
        fmt.Printf("  Updating zone: %s [%d]\n", zoneName, len(dnsList))
        for _, dns := range dnsList {
            list = append(list, dns)
        }
    }

    // Make this the new local list
    WriteList(list, DNSDataFile)
    return
}


// Return all DNS objects for given zone ID, from AWS
func GetDNSListByZoneIdFromAWS(zoneId string) (list []ResourceRecordSetType) {
    SetAWSRegion()
    sess := session.Must(session.NewSession())
    svc := route53.New(sess, aws.NewConfig().WithRegion(AWSRegion))

    params := &route53.ListResourceRecordSetsInput{
        HostedZoneId: aws.String(zoneId),
        MaxItems: aws.String("100"),  // 100 is an AWS limit
    }

    // Loop requests in case there're more than PageSize records or we're being throttled
    for {
        // Get batch of records
        resp, err := svc.ListResourceRecordSets(params)
        if err != nil {
            if BeingThrottled(err) {   // Sleep for a moment if AWS is throttling us
                fmt.Printf("  AWS throttling. Sleeping %d seconds...\n", APISecondsDelay)
                time.Sleep(time.Duration(APISecondsDelay) * time.Second)
                continue
            }
            panic(err.Error())   // Abort on any other error
        }

        // Ensure valid data came back
        if resp.ResourceRecordSets != nil {
            // Decode this batch into our extended type
            // First convert it to raw []byte
            jsonData, err := json.Marshal(resp.ResourceRecordSets)
            if err != nil {
                panic(err.Error())
            }
            // Now read it into extended type list
            var dnsList []ResourceRecordSetType
            err = json.Unmarshal(jsonData, &dnsList)
            if err != nil {
                panic(err.Error())
            }
            // Add this batch to our list
            for _, dns := range dnsList {
                // Add our additional fields
                dns = *dns.SetAccountAlias(AWSAccountAlias)
                dns = *dns.SetAccountId(AWSAccountId)
                dns = *dns.SetZoneId(zoneId)
                list = append(list, dns)
            }
        }

        // Exit loop if no more records, else setup next batch request
        if *resp.IsTruncated == false {
            break
        } else {
            params.StartRecordName = resp.NextRecordName
        }
    }
    return list
}
