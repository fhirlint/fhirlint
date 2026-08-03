Titel: How do you validate FHIR that isn't a clean standalone resource?

(alles ab hier ist der Post-Body)

The official HL7 validator from `org.hl7.fhir.core` is the reference implementation and it's correct, which is the part that matters. But it wants a clean FHIR resource in a file, and real payloads rarely look like that. The resource comes back wrapped in an API envelope, or carries a vendor block that was never going to validate, or trips a constraint the whole project agreed to live with two years ago.

So I wrapped it in a Go CLI called fhirlint. It doesn't reimplement validation, it shells out to the same JAR (downloaded on first run). The two parts I reach for most:

Getting the resource out of whatever it arrived in:

    fhirlint validate api-response.json --extract "$.data.fhir"
    fhirlint validate api-response.json --extract-each "$.medications"
    fhirlint validate patient.json --ignore "$.meta.tag" --ignore "$.text"

`--extract-each` treats each element of a JSON array as its own resource. Same path syntax works on XML, and on `--url` input. Your source file is never touched, preprocessing happens on a copy.

Silencing deviations you've accepted, without switching validation off wholesale:

    fhirlint validate patient.json --suppress messageId:dom-6
    fhirlint validate rx.json --suppress expression:MedicationRequest.intent

In `fhirlint.yml` a suppression can carry a reason and an expiry date, so the list doesn't quietly become permanent.

The rest briefly: JSON, HTML, JUnit and SARIF output, CI exit codes, baseline mode, a warm server mode. Still Java 17+ and the same JAR underneath, FHIR only. Apache-2.0: https://github.com/fhirlint/fhirlint

How are you handling wrapped payloads today? Everyone I ask either has a `jq` step in front of the validator or validates by eye.
