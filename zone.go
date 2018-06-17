// zone.go
package main

import (
    "fmt"
    "time"
    "strings"
    "encoding/json"
    "github.com/aws/aws-sdk-go/aws"
    "github.com/aws/aws-sdk-go/aws/session"
    "github.com/aws/aws-sdk-go/service/route53"
    "github.com/aws/aws-sdk-go/aws/awsutil"
)

// Extend AWS route53.HostedZone type to include these additional fields
type HostedZoneType struct {
    AccountAlias  *string
    AccountId     *string
    *route53.HostedZone
}

// Return string representation of this type
func (s HostedZoneType) String() string {
    return awsutil.Prettify(s)
}

// Set AccountAlias field's value
func (s *HostedZoneType) SetAccountAlias(v string) *HostedZoneType {
    s.AccountAlias = &v
    return s
}

// Set AccountId field's value
func (s *HostedZoneType) SetAccountId(v string) *HostedZoneType {
    s.AccountId = &v
    return s
}


// Display all zones records with applied filter
func ListZones(filter string) {
    list, err := GetZoneList()
    if err != nil {
        Die(1, err.Error())
    }
    for _, zone := range list {
        zoneName, zoneType := "-", "public"
        if zone.Name != nil {
            zoneName = strings.TrimSuffix(*zone.Name, ".")  // Remove useless dotted suffix
        }
        if zone.Config != nil &&
           zone.Config.PrivateZone != nil &&
           *zone.Config.PrivateZone == true {
            zoneType = "private"
        }
        
        accAlias := "-"
        if zone.AccountAlias != nil { accAlias = *zone.AccountAlias }

        // Print all qualifying entries
        if filter == "" || strContains(zoneName, filter) ||
                           strContains(zoneType, filter) ||
                           strContains(accAlias, filter) ||
                           strContains(*zone.Id, filter) {
            fmt.Printf("%-44s  %-8s  %6d  %-30s  %-18s\n", zoneName, zoneType,
                *zone.ResourceRecordSetCount, *zone.Id, accAlias)
        }
    }
    return
}


// Return zone records list in local or remote store
func GetZoneList() (list []HostedZoneType, err error) {
    localFileTimestamp := GetLocalFileTime(ZoneDataFile)
    remoteFileTimestamp := GetRemoteFileTime(ZoneDataFile)

    // Use remote S3 file if it's newer
    if remoteFileTimestamp.After(localFileTimestamp) {
        tmplist, err := GetListFromRemote(ZoneDataFile)
        list = tmplist.([]HostedZoneType)    // Assert our zone type
        if err == nil {
            WriteList(list, ZoneDataFile)   // Update local with this newer set
            return list, nil
        }
        // Return what must be an empty list with the error code
        return list, err
    }

    // Else, just return local file content with error code
    tmplist, err := GetListFromLocal(ZoneDataFile)
    list = tmplist.([]HostedZoneType)
    return list, err
}


// Return list of zoneIDs for zones that have changed within minutesAgo or in the last 7 days
func GetUpdatedZoneIdList(minutesAgo int) (list []string) {
    // Get all route53 Cloudtrail events within minutesAgo
    for _, eventString := range GetCloudTrailEvents("route53", minutesAgo) {
        // The embedded CloudTrailEvent field is a doubly encoded json string whose structure we
        // don't know. Let's convert it to a mapped string, so we can parse it below. This would
        // be easier if the API had a CloudTrailEvent struct type.
        var obj map[string]interface{}
        err := json.Unmarshal([]byte(*eventString.CloudTrailEvent), &obj)
        if err != nil {
            panic(err.Error())
        }

        // Parse the mapped string event object, looking for where the zoneId is
        for k, v := range obj {
            var zoneId string
            // See if the updated zoneId is under requestParameters
            if strings.EqualFold(k, "requestParameters") && v != nil {
                for k2, v2 := range v.(map[string]interface{}) {
                    if strings.EqualFold(k2, "hostedZoneId") {
                        zoneId = "/hostedzone/" + v2.(string)
                        list = AppendIfMissing(list, zoneId)
                    }
                }
            }
            // Else, see if it's under responseElements
            if zoneId == "" && strings.EqualFold(k, "responseElements") && v != nil {
                for k2, v2 := range v.(map[string]interface{}) {
                    if strings.EqualFold(k2, "hostedZone") && v2 != nil {
                        for k3, v3 := range v2.(map[string]interface{}) {
                            if strings.EqualFold(k3, "id") {
                                zoneId = v3.(string)
                                list = AppendIfMissing(list, zoneId)
                            }
                        }
                    }
                }
            }
        }
    }
    return list
}


// Append to list only if element doesnt already exist
func AppendIfMissing(list []string, target string) []string {
    for _, element := range list {
        if strings.EqualFold(element, target) {
            return list
        }
    }
    return append(list, target)
}


// Check if there's a zone with zoneId in given zone list
func IdInZoneList(zoneId string, list []HostedZoneType) bool {
//func zoneListContainsId(list *[]HostedZoneType, zoneId string) bool {
    for _, zone := range list {
        if strings.EqualFold(*zone.Id, zoneId) {
            return true
        }
    }
    return false
}


// Update local copy of DNS zones from current AWS account
func UpdateLocalZoneStoreFromAWS(minutesAgo int) {
    // Do full update if minutesAgo is zero (meaning it wasn't specified)
    if minutesAgo == 0 {
        fmt.Printf("Updating local zone store.\n")
    } else {
        updatedZonesCount := len(GetUpdatedZoneIdList(minutesAgo))
        if updatedZonesCount < 1 {
            // Skip zone update if no route53 zone events within last minutesAgo
            fmt.Printf("Skipping local DNS zone store update (no mods within %d minutes)\n",
                minutesAgo)
            return
        }
        fmt.Printf("Updating local DNS zone store (%d modified within %d minutes)\n",
            updatedZonesCount, minutesAgo)
    }

    // Create a new list from existing store, without the ones for the current AWS account
    var list []HostedZoneType
    zoneList, _ := GetZoneList()
    for _, zone := range zoneList {
        if *zone.AccountId != AWSAccountId {
            list = append(list, zone)
        }
    }

    // Now get all records for this account, and add them to this new list
    for _, zone := range GetZoneListFromAWS() {
        list = append(list, zone)
    }

    // Make this the new local list
    WriteList(list, ZoneDataFile)
    return
}


// Return all zone objects in current AWS account
func GetZoneListFromAWS() (list []HostedZoneType) {
    SetAWSRegion()
    sess := session.Must(session.NewSession())
    svc := route53.New(sess, aws.NewConfig().WithRegion(AWSRegion))

    params := &route53.ListHostedZonesInput{
        MaxItems: aws.String("100"),  // This is an AWS limit
    }

    // Loop requests in case there're more than PageSize records or we're being throttled
    errcount := 0
    for {
        // Get batch of records
        resp, err := svc.ListHostedZones(params)
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
        if resp.HostedZones != nil {
            // Decode this batch into our extended type
            // First convert it to raw []byte
            jsonData, err := json.Marshal(resp.HostedZones)
            if err != nil {
                panic(err.Error())
            }
            // Now read it into extended type list
            var zoneList []HostedZoneType
            err = json.Unmarshal(jsonData, &zoneList)
            if err != nil {
                panic(err.Error())
            }
            // Add this batch to our list
            for _, zone := range zoneList {
                // Add our additional fields
                zone = *zone.SetAccountAlias(AWSAccountAlias)
                zone = *zone.SetAccountId(AWSAccountId)
                list = append(list, zone)
            }
        }

        // Exit loop if no more records, else setup next batch request
        if *resp.IsTruncated == false {
            break
        } else {
            params.Marker = resp.NextMarker
        }
    }
    return list
}
