# AWS CLI Information Utility
This is a system admin utility that allows quick querying of the following AWS resources: EC2 instances, ELBs, R53 DNS zones and records, and CloudFormation stacks. It also allows the breakdown of a DNSRECORD into its backend ELB and instances components. See Usage below for more info.

The speed in querying these services is achieve by storing/caching the info in JSON files that are securely accessible either locally (under the $HOME/.awsinfo/ directory), or remotely in an S3 bucket, or a combination of both. It uses the following 5 files: `inst.json`, `elb.json`, `zone.json`, `dns.json`, and `stack.json`.

Originally this was written in Python, but we decided to rewrite it in Go to play with the AWS SDK http://docs.aws.amazon.com/sdk-for-go/api/ and just to get more familiar with this language.

## Local Store
To use it with Local Store only, you will need to run `awsinfo -u` for every AWS account for which you want to gather and query resources for. The only drawback with this is that the data will eventually experience update drift, and  you will need to rerun `-u` updates again. Although you could automate the update of the local store, it is best to do this in one centralize place which is essentially what Remote Store offers (see below).

## Remote Store
To use it  with remote store you will need to setup a scheduled job to periodically run the `-u` as well as `-3` to copy the files to a secure S3 bucket that you can specify in the `$HOME/.awsinfo/config` file. With this method you will also need to run it against all the AWS accounts for which you want to query resources for. The advantage of remote store is that the data can be more easily updated and managed, and the process can be more easily automated and shared by other system admins in your organization.

Note that with this method the utility will inherently run in *hybrid* mode and it will keep and use local copies of the remote stores. It will download the latest remote files only when it detects they are newer.

To setup the Remote Store run `awsinfo -y` to create the `$HOME/.awsinfo/confing` file, then review the parameters. The variables should be self-explanatory.

NOTE: For security reasons, when you setup the remote S3 bucket make sure you limit HTTP access to *only* internal networks to your organization. The bucket should only be writable by a dedicated account with very limited privileges.

## Getting Started
To compile and install the utility follow below steps. Note that this is only supported on macOS and Linux.
  1. Download the latest version of Go from https://golang.org/dl/
  2. Review and if necessary modify the `Makefile` accordingly
  3. Run `make all`
  4. Put resulting `awsinfo` binary file somewhere in your system path. For those whose binaries reside in `$HOME/data/bin`, you can run `make install`.

## Usage
Once in your PATH, you can use the utility as per usage below.
<pre><code>
AWS CLI Information Utility 2.0.2
awsinfo DNSRECORD        Print IPs/ELB/instances breakdown for given DNSRECORD
        -e  [STRING]     List ELBs, filter with optional STRING
        -d  [STRING]     List DNS records, filter with optional STRING
        -i  [STRING]     List EC2 instances, filter with optional STRING
        -s  [STRING]     List CloudFormation stacks, filter with optional STRING
        -z  [STRING]     List DNS zones, filter with optional STRING
        -h               Show extended options
        -3               Copy local stores to S3 bucket defined in ~/.awsinfo/config
        -3f              Ignore file time stamps and force above copying
        -eh [STRING]     List ELB health-checks, filter with optional STRING
        -es [STRING]     List ELB SSL certs, filter with optional STRING
        -iv [STRING]     List EC2 instances, more verbosely
        -sv [STRING]     List CloudFormation stacks, more verbosely
        -u  [MIN|ZONES]  Update local stores, and only DNS records changed in last MIN minutes,
                         or from zones in ZONES string, e.g., 'mysite.com,a.mydns.com,site.io'
        -x               Delete local store, to start afresh
        -y               Create skeleton ~/.awsinfo/config file
</code></pre>
