# AWS CLI Information Utility
A sysadmin command-line utility that allows quick and dirty querying of the following AWS resources: EC2 instances, ELBs, R53 DNS zones and records, and CloudFormation stacks. It also allows the breakdown of a DNS/ELB endpoint into its instances backends. See below for more info.

The speed in querying these services is achieve by caching the AWS resources info in JSON files stored in the `$HOME/.awsinfo/` directory. The files are called `inst.json`, `elb.json`, `zone.json`, `dns.json`, and `stack.json`.

## Installation
The prefer installation method is with [Homebrew](https://brew.sh):
  * `brew untap lencap/tools ; brew tap lencap/tools` to grab the lastest formula.
  * Then `brew install lencap/tools/awsinfo` or `brew upgrade lencap/tools/awsinfo`

Alternatively, you can compile and install manually:  
  * Install GoLang (please find out how that's done somewhere else).
  * Run `make all` if compiling for the first time, or just `make` if it's a subsequent compile. 
  * Install the resulting `awsinfo` binary somewhere in your PATH.

Next, setup the `$HOME/.awsinfo/config` file:
  * `awsinfo -y`
  * `vi $HOME/.awsinfo/config` and replace the default **awsinfo** with the actual S3 bucket name for your organization.

## Local Store
To use it with Local Store only, you will need to CLI logon to each respective AWS account and run `awsinfo -u`. This will gather the records of all those resources and store them locally in the files mentioned above. The drawback with this method is that the data will eventually get old, and you will need to rerun `-u` updates again and again. Although you could automate this update locally, it is best to do this in a centralize place which is essentially what Remote Store offers (see below).

## Remote Store
To use it with Remote Store you will need to setup a scheduled job to periodically run the `-u` update (as well as `-3` to actually copy the files) to a secure S3 bucket that you can specify in the `$HOME/.awsinfo/config` file. With this method you will also need to run it against all the AWS accounts for which you want to query resources for. The advantage of Remote Store is that the data can be more easily updated and managed, and the process can be more easily automated and shared by other sysadmins in your organization.

Note that with this method the utility will inherently run in *hybrid* mode and it will keep and use local copies of the Remote Stores. It will download the latest remote files **only** when it detects they are newer.

NOTE: For security reasons, when you setup the remote S3 bucket make sure you limit HTTP access to **only** internal networks to your organization. Also, the bucket should only be writable by a dedicated account with very limited privileges.

## Usage
Once in your PATH, you can use the utility as per usage below.
<pre><code>
$ awsinfo -h
AWS CLI Information Utility 2.0.9
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
        -dv [STRING]     List DNS records, more verbosely
        -iv [STRING]     List EC2 instances, more verbosely
        -sv [STRING]     List CloudFormation stacks, more verbosely
        -u  [MIN|ZONES]  Update local stores, and only DNS records changed in last MIN minutes,
                         or from zones in ZONES string, e.g., 'mysite.com,a.mydns.com,site.io'
        -x               Delete local store, to start afresh
        -y               Create skeleton ~/.awsinfo/config file
</code></pre>
