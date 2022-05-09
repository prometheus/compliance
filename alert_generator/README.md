# Prometheus Alert Generator Compliance

## Specification

You can find the specification for this compliance [here](specification.md).

## Running the tests

#### Step 1

Clone the repo
```bash
$ git clone https://github.com/prometheus/compliance.git
$ cd compliance/alert_generator
```

#### Step 2

Check that the rules file is up-to-date with

```bash
$ make check-rules
go run ./cmd/rule_config_builder/main.go -rules-file-path="./rules.yaml"
level=info ts=2022-02-24T14:14:19.596Z caller=main.go:53 msg="Rules file successfully generated" path=/home/ganesh/go/src/github.com/prometheus/compliance/alert_generator/rules.yaml

$ echo $?
0
```

Run `make rules` otherwise to generate fresh rules file.

Feed the `rules.yaml` file present in the directory into the software that you are testing with any additional tweaks required.

It is suggested to wait until one evaluation of all rule groups is done before starting the test.

#### Step 3

Check [test-example.yaml](test-example.yaml) for the example config to setup the test suite. That file has all the documentation on how to configure the test suite.

Feel free to modify the example config itself. Config for some softwares are provided as an example, more will be added.

#### Step 4

If you are running your software in a local environment, you can set the alertmanager URL to `http://<host>:<port>` with port being the one set in the test suite config. For example `http://localhost:8080`.

If you are testing a cloud offering, or if the local software setup cannot access the test suite's network, there are two alternatives:

##### Step 4a

You can run `./cmd/alert_generator_compliance_tester` in docker and run it inside your infrastructure as a batch job. To do so, build the image with `make docker` and instead of step 6, simply run `alert_generator_compliance_tester:latest` with `-config-file=<config file you have done in step 3>`.

See example [here](https://github.com/thanos-io/thanos/blob/89077c9df109dac8dbe6c979ebc181071c5e0db8/test/e2e/compatibility_test.go#L132) 

##### Step 4b

1. Open https://webhook.site/. It will give a unique link to use as a webhook. Let's take https://webhook.site/12345 as an example.
2. Set https://webhook.site/12345 to be the alertmanager URL in your software - hence all alerts will be set to this webhook.
3. On the webhook.site page, enable `XHR Redirect` on top, and in its settings, set the `Target` to `http://<host>:<port>` from the above, for example `http://localhost:8080`.

Note:
* The webhook.site page must be open at all times. It redirects the request locally, hence able to redirect to local network.
* In the test logs you might see "error in unmarshaling" error immediately followed by "alerts received" log. You can ignore the error log. It is something to do with how webhook.site redirects.
* Disable any kind of browser blockers for webhook.site if you are not getting alerts redirected to the test suite.

#### Step 5

If your software send alerts in a format that is not parsable by any of the provided parsers in [alert_message_parsers.go](./alert_message_parsers.go), you can extend that file to include your custom parser and mention that name in the config file.

#### Step 6

Now that everything is set up, you can run the test as follows

```bash
$ make run CONFIG=./test-example.yaml
```
A successful test run will exit with `Congrats! All tests passed` message. An unsuccessful run will end with a description of what went wrong.

**The test takes roughly 40 mins to run.** But if all rule groups face any errors, the test will exit immediately.

---

## Running on Prometheus as an example

1. Get the rules file from step 2 above and put it in the Prometheus config.
2. Set the alertmanager URL to `localhost:8080`.
3. Run Prometheus with `--web.enable-remote-write-receiver` flag to accept remote write.
4. Run the test with `make run CONFIG=./test-prometheus.yaml`
