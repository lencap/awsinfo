// cloudtrail.go
package main

import (
    "fmt"
    "time"
    "strings"
    "github.com/aws/aws-sdk-go/aws"
    "github.com/aws/aws-sdk-go/aws/session"
    "github.com/aws/aws-sdk-go/service/cloudtrail"
)


// Get all CloudTrail events for given AWS source, within last minutes_ago or 7 days ago
func GetCloudTrailEvents(source string, minutesAgo int) (list []*cloudtrail.Event) {
    SetAWSRegion()
    sess := session.Must(session.NewSession())
    svc := cloudtrail.New(sess, aws.NewConfig().WithRegion(AWSRegion))
    source = source + ".amazonaws.com"

    startTime := time.Now().UTC()
    if minutesAgo == 0 {
        startTime = startTime.AddDate(0,0,-7)   // Default to 7 days (10080 mins) ago
    } else {
        startTime = startTime.Add(-time.Duration(minutesAgo) * time.Minute)
    }

    params := &cloudtrail.LookupEventsInput{
        LookupAttributes: []*cloudtrail.LookupAttribute{
            {
                AttributeKey:   aws.String("EventSource"),
                AttributeValue: aws.String(source),
            },
        },
        MaxResults: aws.Int64(50),            // This is an AWS limit
        StartTime: aws.Time(startTime),
        EndTime: aws.Time(time.Now().UTC()),  // AWS API uses UTC
    }

    // UNCOMMENT TO HELP DEBUGGING
    //fmt.Printf("Checking CloudTrail for updates in source: %s\n", source)

    // Loop requests, in case there are more than maxResults records or AWS is throttling
    errcount := 0
    for {
        // Get batch of records
	    resp, err := svc.LookupEvents(params)
        if err != nil {
            // Sleep for a moment if AWS is throttling us
            if BeingThrottled(err) {
                fmt.Printf("AWS throttling. Sleeping %d seconds...\n", APISecondsDelay)
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

        for _, event := range resp.Events {  // Add this batch to the list
            // We don't care about 'List' or 'Describe' event types
            if strings.Contains(*event.EventName, "List") ||
               strings.Contains(*event.EventName, "Describe") {
                continue
            }
            // UNCOMMENT TO HELP DEBUGGING
            // fmt.Println(source, *event.EventName)
            
            list = append(list, event)
        }

        if resp.NextToken == nil {   // Exit loop if no more records, 
            break
        } else {
            params.NextToken = resp.NextToken  // else setup next batch request
        }

    }
    return list
}
