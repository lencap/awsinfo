// stack.go
package main

import (
    "fmt"
    "time"
    "strings"
    "encoding/json"
    "github.com/aws/aws-sdk-go/aws"
    "github.com/aws/aws-sdk-go/aws/session"
    "github.com/aws/aws-sdk-go/service/cloudformation"
    "github.com/aws/aws-sdk-go/aws/awsutil"
)


// Extend AWS route53.HostedZone type to include these additional fields
type StackType struct {
    AccountAlias  *string
    AccountId     *string
    *cloudformation.Stack
}

// Return string representation of this type
func (s StackType) String() string {
    return awsutil.Prettify(s)
}

// Set AccountAlias field's value
func (s *StackType) SetAccountAlias(v string) *StackType {
    s.AccountAlias = &v
    return s
}

// Set AccountId field's value
func (s *StackType) SetAccountId(v string) *StackType {
    s.AccountId = &v
    return s
}


// Display all stack records with applied filter
func ListStacks(filter, option string) {
    stkList, err := GetStackList()
    if err != nil {
        Die(1, err.Error())
    }
    for _, stkRec := range stkList {
        stkName, acctAlias, stkStatus, stkId, lastUpdate := "-", "-", "-", "-", "-"
        if stkRec.StackStatus == nil {
            panic("Error. This stack record is missing field StackStatus.")
        } else {
    		stkStatus = *stkRec.StackStatus
    		if strContains(stkStatus, "delete_complete") {
	            continue   // Skip. We only care about active stacks 
    		}
    	}
        if stkRec.StackName != nil { stkName = *stkRec.StackName }
        if stkRec.AccountAlias != nil { acctAlias = *stkRec.AccountAlias }
        if stkRec.StackId != nil { stkId = *stkRec.StackId }
	    if stkRec.LastUpdatedTime != nil {
	        lu := *stkRec.LastUpdatedTime
	        lastUpdate = lu.Format("2006-01-02 15:04")
	    }	
        if filter == "" || strContains(stkName, filter) ||
                           strContains(stkId, filter) ||
                           strContains(acctAlias, filter) ||
                           strContains(stkStatus, filter) {

            //  Replace spaces with period and shorten names
            if len(stkName) > 50 { stkName = stkName[:50] }
            stkName = strings.Replace(stkName, " ", ".", -1)

            fmt.Printf("%-50s  %-22s  %-24s  %s\n", stkName, acctAlias, stkStatus, lastUpdate)
            if option == "-sv" {  // List parameters if extra verbosity was requested
    	        if stkRec.Parameters != nil {
    	        	for _, p := range stkRec.Parameters {
                        if p.ParameterKey != nil && p.ParameterValue != nil {
                            fmt.Printf("  %-32s  %s\n", *p.ParameterKey, *p.ParameterValue)
                        }
    	        	}
    	        }
            }
        }
    }
    return
}


// Return stack records list in local or remote store
func GetStackList() (list []StackType, err error) {
    localFileTimestamp := GetLocalFileTime(StackDataFile)
    remoteFileTimestamp := GetRemoteFileTime(StackDataFile)

    // Use remote S3 file if it's newer
    if remoteFileTimestamp.After(localFileTimestamp) {
        tmplist, err := GetListFromRemote(StackDataFile)
        list = tmplist.([]StackType)         // Assert our stack type
        if err == nil {
            WriteList(list, StackDataFile)   // Update local with this newer set
            return list, nil
        }        
        // Return what must be an empty list with the error code
        return list, err
    }

    // Else, just return local file content with error code
    tmplist, err := GetListFromLocal(StackDataFile)
    list = tmplist.([]StackType)
    return list, err
}


// Update local stack store from current AWS account
func UpdateLocalStackStoreFromAWS(minutesAgo int) {
    // Do full update if minutesAgo is zero (meaning it wasn't specified)
    if minutesAgo == 0 {
        fmt.Printf("Updating local CloudFormation stack store\n")
    } else {
        updatedStackCount := len(GetCloudTrailEvents("cloudformation", minutesAgo)) 
        if updatedStackCount < 1 {
            // Skip stack update if no cloudformation events within last minutesAgo
            fmt.Printf("Skipping local CloudFormation stack store update (no mods within %d minutes)\n",
                minutesAgo)
            return
        }
        fmt.Printf("Updating local CloudFormation stack store (%d modified within %d minutes)\n",
            updatedStackCount, minutesAgo)
    }

    // Create a new list from existing store, without the ones for the current AWS account
    var list []StackType
    stackList, _ := GetStackList()
    for _, stack := range stackList {
        if *stack.AccountId != AWSAccountId {
            list = append(list, stack)
        }
    }

    // Now get all records for this account, and add them to this new list
    for _, stack := range GetStackListFromAWS() {
        list = append(list, stack)
    }

    // Make this the new local list
    WriteList(list, StackDataFile)
    return
}


// Return all stack objects in current AWS account
func GetStackListFromAWS() (list []StackType) {
    SetAWSRegion()
    sess := session.Must(session.NewSession())
    svc := cloudformation.New(sess, aws.NewConfig().WithRegion(AWSRegion))

    params := &cloudformation.DescribeStacksInput{}

    // Loop requests in case there're more than PageSize records or we're being throttled
    errcount := 0
    for {
        // Get batch of records
        resp, err := svc.DescribeStacks(params)
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
        if resp.Stacks != nil {
            // Decode this batch into our extended type
            // First convert it to raw []byte
            jsonData, err := json.Marshal(resp.Stacks)
            if err != nil {
                panic(err.Error())
            }
            // Now read it into extended type list
            var stkList []StackType
            err = json.Unmarshal(jsonData, &stkList)
            if err != nil {
                panic(err.Error())
            }
            // Add this batch to our list
            for _, stk := range stkList {
                // Add our additional fields
                stk = *stk.SetAccountAlias(AWSAccountAlias)
                stk = *stk.SetAccountId(AWSAccountId)
                list = append(list, stk)
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
