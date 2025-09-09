As part of my team's on-call rotation, we are responsible for reading and responding to failures in our testing GitHub workflows. Issues are created in the public GitHub repo radius-project/radius. The following is a real-world example of the email that I draft every week:

```
Hi team,
 
Here is the summary of Radius on-call from June 23rd – June 30th 2025.

Summary
* Release 0.48 completed
  * Fixed release verification workflow issue: https://github.com/radius-project/radius/pull/9862

* Dependabot PRs merged
  * https://github.com/radius-project/radius/pull/9846
  * https://github.com/radius-project/radius/pull/9813
  * https://github.com/radius-project/radius/pull/9826
  * https://github.com/radius-project/radius/pull/9847
  * https://github.com/radius-project/radius/pull/9845

* Functional (noncloud) test failures: (3/39, 8%) (Query: https://github.com/radius-project/radius/actions/workflows/functional-test-noncloud.yaml?query=event%3Aschedule+created%3A2025-06-23..2025-06-30+is%3Afailure)
  * (x2): Gitea install failed: https://github.com/radius-project/radius/issues/9894
  * (x1): DynamicRP workspace issue: https://github.com/radius-project/radius/issues/9895

* Functional (cloud) test failures: (11/39, 28%) (Query: https://github.com/radius-project/radius/actions/workflows/functional-test-cloud.yaml?query=event%3Aschedule+created%3A2025-06-23..2025-06-30+is%3Afailure)
  * (x1): Contour Helm install: Probably transient, did not file a tracking issue.
  * (x10): Test database migration: GitHub secrets issue with the new test database. Fixed as of this morning.

* Samples tests failures: (1/7, 14%) (Query: https://github.com/radius-project/samples/actions/workflows/test.yaml?query=event%3Aschedule+created%3A2025-06-23..2025-06-30+is%3Afailure)
  * docker.io ratelimit

* Long-running tests failures: (19/47, 40%) (Query: https://github.com/radius-project/radius/actions/workflows/long-running-azure.yaml?query=event%3Aschedule+created%3A2025-06-23..2025-06-30+is%3Afailure)
  * (x18): Test database migration.
  * (x1): Timeout on Test_DaprPubSubBroker_Manual_Secret: https://github.com/radius-project/radius/issues/9896

* Customer/ community issues: 0


Thanks,
Will S
```

Your role is a helpful assistant which will update these links to the current week and perform web searches to populate the data and root causes. Please link open issues in the correct repos if necessary, and if none look reasonable, place a TODO there.