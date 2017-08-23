// main.go
package main

import (
    "fmt"
    "os"
    "path/filepath"
    "strings"
    "strconv"
    "time"
    "errors"
    "encoding/json"
    "io/ioutil"
    "net/http"
    "github.com/vaughan0/go-ini"
    "github.com/aws/aws-sdk-go/aws"
    "github.com/aws/aws-sdk-go/aws/session"
    "github.com/aws/aws-sdk-go/service/iam"
    "github.com/aws/aws-sdk-go/service/ec2"
    "github.com/aws/aws-sdk-go/service/s3/s3manager"
)


// Exit program and print final message
func Die(code int, message string) {
    if message != "" {
        fmt.Println(message)
    }
    os.Exit(code)
}


// Write generic JSON object list to local file
func WriteList(jsonObject interface{}, storeFile string) {
    // The generic interface{} allows us to write list of any types
    localFile := filepath.Join(progConfDir, storeFile)  // Note progConfDir is global
    jsonData, err := json.Marshal(jsonObject)
    if err != nil {
        panic(err.Error())
    }
    err = ioutil.WriteFile(localFile, jsonData, 0600)
    if err != nil {
        panic(err.Error())
    }
}


// Set up global variables as per configuration file
func ProcessConfigFile() {
    // Ensure config directory exist
    progConfDir = filepath.Join(os.Getenv("HOME"), "." + ProgName)
    if _, err := os.Stat(progConfDir); os.IsNotExist(err) { // Create it if it doesnt exist
        err := os.Mkdir(progConfDir, 0700)
        if err != nil {
            panic(err.Error())
        }
    }

    // Set up global variables based on program config file
    confFile := filepath.Join(progConfDir, "config")
    if _, err := os.Stat(confFile); os.IsNotExist(err) {
        return  // File doesn't exist so we'll use the default hard-coded global variables
    } else {
        cfgfile, err := ini.LoadFile(confFile)
        if err != nil {
            panic(err.Error())
        }
        tmpS3Bucket, _ := cfgfile.Get("default", "s3_bucket")
        if tmpS3Bucket == "" {
            Die(1, "Error. s3_bucket not defined in " + confFile)
        }
        S3Bucket = tmpS3Bucket
        tmpS3URLBase, _ := cfgfile.Get("default", "s3_url_base")
        if tmpS3URLBase == "" {
            Die(1, "Error. s3_url_base not defined in " + confFile)
        }
        S3URLBase = tmpS3URLBase
        tmpAPISecondsDelay, _ := cfgfile.Get("default", "api_seconds_delay")
        if tmpAPISecondsDelay == "" {
            Die(1, "Error. api_seconds_delay not defined in " + confFile)
        }
        APISecondsDelay, _ = strconv.Atoi(tmpAPISecondsDelay)
        tmpR53APISecondsDelay, _ := cfgfile.Get("default", "r53_api_seconds_delay")
        if tmpR53APISecondsDelay == "" {
            Die(1, "Error. r53_api_seconds_delay not defined in " + confFile)
        }
        R53APISecondsDelay, _ = strconv.Atoi(tmpR53APISecondsDelay)
    }
}


// Create a skeleton configuration file with default hard-coded values
func CreateSkeltonConfigFile() {
    confFile := filepath.Join(progConfDir, "config")
    if _, err := os.Stat(confFile); os.IsNotExist(err) {
        content := "# Edit these values to match your environment setup\n"
        content += "[default]\n"
        content += "s3_bucket = " + S3Bucket + "\n"
        content += "s3_url_base = " + S3URLBase + "\n"
        content += "api_seconds_delay = " + strconv.Itoa(APISecondsDelay) + "\n"
        content += "r53_api_seconds_delay = " + strconv.Itoa(R53APISecondsDelay) + "\n"
        err = ioutil.WriteFile(confFile, []byte(content), 0600)
        if err != nil {
            panic(err.Error())
        }        
    } else {
        fmt.Printf("There's already a %s file.\n", confFile)
    }
}


// Set AWS account ID and Alias, which also implicitly tests whether user is logged in or not
func SetupAWSAccess() {
    SetAWSRegion()

    sess := session.Must(session.NewSession())

    // Set AWS account ID globally
    svc := ec2.New(sess, aws.NewConfig().WithRegion(AWSRegion))
    resp, err := svc.DescribeSecurityGroups(&ec2.DescribeSecurityGroupsInput{})
    if err == nil {
        AWSAccountId = *resp.SecurityGroups[0].OwnerId
        // There's no SDK call to find the AccountId, but default OwnerId of all SG is it
    } else {
        fmt.Println(err.Error())        
    }

    // Set AWS account alias globally
    svc2 := iam.New(sess, aws.NewConfig().WithRegion(AWSRegion))
    resp2, err2 := svc2.ListAccountAliases(&iam.ListAccountAliasesInput{})
    if err2 == nil {
        AWSAccountAlias = *resp2.AccountAliases[0]
    } else {
        fmt.Println(err2.Error())
    }
    
    if AWSAccountId == "" || AWSAccountAlias == "" {
        fmt.Println("AWS login is required.")
        os.Exit(1)
    }
}


// Copy local store files to S3 bucket area defined in config file
func CopyLocalStoresToS3Bucket(option string) {
    SetAWSRegion()
    sess := session.Must(session.NewSession())
    uploader := s3manager.NewUploader(sess)

    fileList := []string{DNSDataFile, ZoneDataFile, ELBDatafile, InstanceDataFile, StackDataFile}

    for _, file := range fileList {
        localFileTimestamp := GetLocalFileTime(file)
        remoteFileTimestamp := GetRemoteFileTime(file)
        // Update S3 copy only if local one is newer or we have the Force option
        if localFileTimestamp.After(remoteFileTimestamp) || option == "-3f" {
            // Open local file
            localFile := filepath.Join(progConfDir, file)
            f, err  := os.Open(localFile)
            if err != nil {
                panic(err.Error())
            }
            // Upload to S3
            params := &s3manager.UploadInput{
                Bucket: aws.String(S3Bucket),   // S3Bucket is a global variable
                Key:    aws.String(file),
                Body:   f,
            }
            result, err := uploader.Upload(params)
            if err != nil {
                panic(err.Error())
            }
            fmt.Printf("Remote upload to %s\n", aws.StringValue(&result.Location))
        } else {
            fmt.Printf("Skipping %s. The S3 copy is newer than local one.\n", file)
        }     
    }
}


// Delete, clean up the local store files
func DeleteLocalStoresFiles(option string) {
    fileList := []string{DNSDataFile, ZoneDataFile, ELBDatafile,InstanceDataFile,StackDataFile}
    for _, file := range fileList {
        localFile := filepath.Join(progConfDir, file)
        os.Remove(localFile)
    }
}


// Set AWS region globally
func SetAWSRegion() {
    // Exit if unable to find a way to set the AWSRegion global variable
    AWSRegion = ""
    // Start by checking the environment variables (order is important)
    if os.Getenv("AWS_REGION") != "" {
        AWSRegion = os.Getenv("AWS_REGION")
    } else if os.Getenv("AMAZON_REGION") != "" {
        AWSRegion = os.Getenv("AMAZON_REGION")
    } else if os.Getenv("AWS_DEFAULT_REGION") != "" {
        AWSRegion = os.Getenv("AWS_DEFAULT_REGION")
    } else {
        // End by checking the AWS config file
        AWSConfigFile := filepath.Join(os.Getenv("HOME"), ".aws/config")
        if _, err := os.Stat(AWSConfigFile); os.IsNotExist(err) {    
            fmt.Printf("AWS_REGION variable is not defined, and %s file does not exist.\n", AWSConfigFile)
            os.Exit(1)
        }
        cfgfile, err := ini.LoadFile(AWSConfigFile)
        if err != nil {
            fmt.Println(err.Error())
            os.Exit(1)
        }
        AWSRegion, _ = cfgfile.Get("default", "region") // Note global var assignment
    }
    if AWSRegion == "" {
        fmt.Printf("You must specify a region. You can also configure your region by running \"aws configure\".\n")
        os.Exit(1)
    }
    return
}


// Case insensitive version of strings.Contains()
func strContains(str string, arg string) bool {
    if strings.Contains(strings.ToLower(str), strings.ToLower(arg)) {
        return true
    }
    return false
}


// Check for AWS throttling
func BeingThrottled(err error) bool {
    if strContains(err.Error(), "Throttl") || strContains(err.Error(), "exceed") {
        return true
    }
    return false
}


// Check if string element is in string list 
func strInList(element string, list []string) bool {
    for _, str := range list {
        if strings.EqualFold(str, element) {
            return true
        }
    }
    return false
}


// Return local file time in UTC
func GetLocalFileTime(dataFile string) (t time.Time) {
    localFile := filepath.Join(progConfDir, dataFile)
    fileinfo, err := os.Stat(localFile)
    if err != nil {
        return t  // Return zero time
    }
    return fileinfo.ModTime().UTC()
}


// Return remote file time in UTC
func GetRemoteFileTime(dataFile string) (t time.Time) {
    S3FileUrl := S3URLBase + "/" + dataFile
    resp, err := http.Head(S3FileUrl)
    if err == nil && resp.StatusCode == 200 {
        lastModifiedDate := resp.Header.Get("Last-Modified")
        lmt, err := time.Parse(time.RFC1123, lastModifiedDate)
        if err == nil {
            return lmt.UTC()
        }
    }
    // If anything, just return zero time
    return t
}


// Return list in local store
func GetListFromLocal(dataFile string) (list interface{}, err error) {
    localFile := filepath.Join(progConfDir, dataFile)  // progConfDir is global
    jsonData, err := ioutil.ReadFile(localFile)
    if err != nil {
        // Return empty list of respective type
        list, _ := GetListFromJSONData(dataFile, &jsonData)
        return list, errors.New(fmt.Sprintf("Can't read file %s", localFile))
    }
    // Return specific type list with error
    return GetListFromJSONData(dataFile, &jsonData)
}


// Return list in remote store
func GetListFromRemote(dataFile string) (list interface{}, err error) {
    S3FileUrl := S3URLBase + "/" + dataFile
    resp, err := http.Get(S3FileUrl)
    if err != nil {
        // Return empty interface list with error
        return list, errors.New(fmt.Sprintf("Can't http.Get %s", S3FileUrl))
    } else {
        if resp.StatusCode == 200 {
            jsonData, err := ioutil.ReadAll(resp.Body)
            if err != nil {
                return list, errors.New(fmt.Sprintf("Can't read body of %s", S3FileUrl))
            }
            // Return specific type list with error
            return GetListFromJSONData(dataFile, &jsonData)
        }
        // Return empty list with error
        return list, errors.New(fmt.Sprintf("URL %s returns a non-200 error", S3FileUrl))
    }
}


// Return list in json data
func GetListFromJSONData(dataFile string, jsonData *[]byte) (list interface{}, err error) {
    switch dataFile {
    case InstanceDataFile: // Return list of instance records
        var list []InstanceType
        err := json.Unmarshal(*jsonData, &list)
        if err != nil {
            return list, errors.New(fmt.Sprintf("Can't unmarshal %s", dataFile))
        }
        return list, err
    case DNSDataFile:      // Return list of DNS records
        var list []ResourceRecordSetType
        err := json.Unmarshal(*jsonData, &list)
        if err != nil {
            return list, errors.New(fmt.Sprintf("Can't unmarshal %s", dataFile))
        }
        return list, err
    case ELBDatafile:      // Return list of ELB records
        var list []LoadBalancerDescriptionType
        err := json.Unmarshal(*jsonData, &list)
        if err != nil {
            return list, errors.New(fmt.Sprintf("Can't unmarshal %s", dataFile))
        }
        return list, err
    case ZoneDataFile:     // Return list of zone records
        var list []HostedZoneType
        err := json.Unmarshal(*jsonData, &list)
        if err != nil {
            return list, errors.New(fmt.Sprintf("Can't unmarshal %s", dataFile))
        }
        return list, err
    case StackDataFile:    // Return list of stack records
        var list []StackType
        err := json.Unmarshal(*jsonData, &list)
        if err != nil {
            return list, errors.New(fmt.Sprintf("Can't unmarshal %s", dataFile))
        }
        return list, err
    default:               // Return list of generic JSON records
        var list interface{}
        err := json.Unmarshal(*jsonData, &list)
        if err != nil {
            return list, errors.New(fmt.Sprintf("Can't unmarshal %s", dataFile))
        }
        return list, err
    }
}
