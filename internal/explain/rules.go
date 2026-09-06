package explain

// rules is the built-in set of explanations, keyed by lowercase message ID.
// It currently covers the most common FHIR R4 core invariants. Profile-specific
// IDs can be added incrementally. Keep keys lowercase — Lookup lowercases input.
var rules = map[string]Rule{
	"dom-2": {
		ID:        "dom-2",
		Title:     "A contained resource SHALL NOT contain nested resources",
		DefinedIn: "FHIR R4 Core (invariant on DomainResource)",
		Description: "When a resource is contained inside another resource (in the\n" +
			"`contained` array), it must not itself carry contained resources.\n" +
			"Nesting contained resources more than one level deep is not allowed.",
		HowToFix: "Move the deeply-nested resource out so it is referenced directly,\n" +
			"or promote it to a standalone resource and reference it by URL.",
	},
	"dom-3": {
		ID:        "dom-3",
		Title:     "A contained resource SHALL be referenced from the container",
		DefinedIn: "FHIR R4 Core (invariant on DomainResource)",
		Description: "Every entry in `contained` must be referenced from somewhere in the\n" +
			"containing resource (or refer back to it). Unreferenced contained\n" +
			"resources are dead weight and are rejected.",
		HowToFix: "Add a reference (e.g. \"#local-id\") to the contained resource from the\n" +
			"appropriate element, or remove the contained resource if it is unused.",
	},
	"dom-4": {
		ID:        "dom-4",
		Title:     "A contained resource SHALL NOT have meta.versionId or meta.lastUpdated",
		DefinedIn: "FHIR R4 Core (invariant on DomainResource)",
		Description: "Contained resources have no independent identity, so version and\n" +
			"last-updated metadata are meaningless for them and must be omitted.",
		HowToFix: "Remove `meta.versionId` and `meta.lastUpdated` from the contained\n" +
			"resource.",
	},
	"dom-5": {
		ID:        "dom-5",
		Title:     "A contained resource SHALL NOT have a security label",
		DefinedIn: "FHIR R4 Core (invariant on DomainResource)",
		Description: "Security labels (`meta.security`) apply to independently-managed\n" +
			"resources. A contained resource inherits the container's context and\n" +
			"must not carry its own security labels.",
		HowToFix: "Remove `meta.security` from the contained resource; apply the label to\n" +
			"the containing resource instead if needed.",
	},
	"dom-6": {
		ID:        "dom-6",
		Title:     "A resource should have narrative for robust management",
		DefinedIn: "FHIR R4 Core (best-practice invariant on DomainResource)",
		Description: "Every DomainResource should contain a human-readable narrative in the\n" +
			"`text` field (a Narrative with status and div). Without it, systems\n" +
			"that cannot parse structured data have no fallback representation.\n" +
			"This is a best-practice constraint, reported as a warning by default.",
		HowToFix: "Add a `text` field to your resource:\n\n" +
			"  {\n" +
			"    \"text\": {\n" +
			"      \"status\": \"generated\",\n" +
			"      \"div\": \"<div xmlns=\\\"http://www.w3.org/1999/xhtml\\\">...</div>\"\n" +
			"    }\n" +
			"  }\n\n" +
			"Or relax best-practice handling globally with --best-practice ignore.",
	},
	"ele-1": {
		ID:        "ele-1",
		Title:     "All FHIR elements must have a @value or children",
		DefinedIn: "FHIR R4 Core (invariant on Element)",
		Description: "Every element must carry either a primitive value, child elements, or\n" +
			"extensions. Empty elements (e.g. \"\" or {}) convey no information and\n" +
			"are not permitted.",
		HowToFix: "Provide a value or children for the element, or remove the empty\n" +
			"element entirely.",
	},
	"ext-1": {
		ID:        "ext-1",
		Title:     "An extension must have either extensions or value[x], not both",
		DefinedIn: "FHIR R4 Core (invariant on Extension)",
		Description: "An extension is either a simple extension (it has a value[x]) or a\n" +
			"complex extension (it has nested sub-extensions). It cannot have both,\n" +
			"and it must have at least one.",
		HowToFix: "For a simple extension, keep `value[x]` and remove nested extensions.\n" +
			"For a complex extension, keep the nested `extension` array and remove\n" +
			"`value[x]`.",
	},
	"bdl-1": {
		ID:        "bdl-1",
		Title:     "total only when a search or history",
		DefinedIn: "FHIR R4 Core (invariant on Bundle)",
		Description: "Bundle.total is only meaningful for searchset and history bundles,\n" +
			"where it reports the total number of matches. It must be absent for\n" +
			"other bundle types (transaction, batch, document, message, collection).",
		HowToFix: "Remove `Bundle.total` unless `Bundle.type` is `searchset` or `history`.",
	},
	"bdl-7": {
		ID:        "bdl-7",
		Title:     "FullUrl must be unique in a bundle",
		DefinedIn: "FHIR R4 Core (invariant on Bundle)",
		Description: "Each entry.fullUrl in a bundle must be unique, unless entries with the\n" +
			"same fullUrl carry different meta.versionId values (a version history).",
		HowToFix: "Ensure each entry has a distinct `fullUrl`, or differentiate duplicate\n" +
			"fullUrls by setting distinct `meta.versionId` values.",
	},
	"bdl-9": {
		ID:        "bdl-9",
		Title:     "A document must have an identifier with a system and a value",
		DefinedIn: "FHIR R4 Core (invariant on Bundle)",
		Description: "Document bundles (Bundle.type = document) must be uniquely identifiable,\n" +
			"so Bundle.identifier is required and must include both a system and a\n" +
			"value.",
		HowToFix: "Add `Bundle.identifier` with both `system` and `value` set when\n" +
			"`Bundle.type` is `document`.",
	},
	"bdl-11": {
		ID:        "bdl-11",
		Title:     "A document must have a Composition as the first resource",
		DefinedIn: "FHIR R4 Core (invariant on Bundle)",
		Description: "In a document bundle, the first entry must be a Composition resource,\n" +
			"which acts as the document's table of contents and provides its\n" +
			"attestation and structure.",
		HowToFix: "Make the first `entry.resource` a Composition when `Bundle.type` is\n" +
			"`document`.",
	},
	"bdl-12": {
		ID:        "bdl-12",
		Title:     "A message must have a MessageHeader as the first resource",
		DefinedIn: "FHIR R4 Core (invariant on Bundle)",
		Description: "In a message bundle, the first entry must be a MessageHeader resource,\n" +
			"which describes the message event, source, and destination.",
		HowToFix: "Make the first `entry.resource` a MessageHeader when `Bundle.type` is\n" +
			"`message`.",
	},
	"unknown_codesystem": {
		ID:        "UNKNOWN_CODESYSTEM",
		Title:     "The code system is not available, so the code was not checked",
		DefinedIn: "HL7 Validator (terminology resolution)",
		Description: "The validator could not obtain a definition for the code system, so\n" +
			"it checked nothing in it. Like the size-limit messages, this reports\n" +
			"a check that did not run rather than a problem with your resource:\n" +
			"a clean result here is not evidence that the code is valid.\n\n" +
			"It arrives as a warning, and the resource still counts as valid, so\n" +
			"a run can look green while none of its codes were verified.\n\n" +
			"For German projects this is the default state, not a\n" +
			"misconfiguration. de.basisprofil.r4 declares ICD-10-GM, OPS, ATC and\n" +
			"Alpha-ID with content=not-present, meaning it defines the code\n" +
			"systems but ships none of their concepts. The content is published\n" +
			"only as bfarm.terminologien.* packages on the BfArM's own registry,\n" +
			"behind a token. It is not on packages.fhir.org, and neither\n" +
			"tx.fhir.org nor tx.fhir.de serves those code systems.",
		HowToFix: "There is no public server to point at, so there are two routes.\n\n" +
			"Install the terminology package into ~/.fhir/packages yourself,\n" +
			"from https://terminologien.bfarm.de/, and name it like any other\n" +
			"IG:\n\n" +
			"  fhirlint validate ./fhir/ --ig bfarm.terminologien.icd10gm#<version>\n\n" +
			"Or run a terminology server loaded with the content and point at\n" +
			"it. A local one over plain HTTP needs --allow-insecure-tx:\n\n" +
			"  fhirlint validate ./fhir/ \\\n" +
			"    --terminology-server http://localhost:8080/fhir --allow-insecure-tx\n\n" +
			"If neither is available, know that the codes are unchecked rather\n" +
			"than believing a green run. fhirlint counts these separately in the\n" +
			"summary for that reason.",
	},
	"valueset_inc_too_many_codes": {
		ID:        "VALUESET_INC_TOO_MANY_CODES",
		Title:     "Too many codes in a ValueSet include to check them all",
		DefinedIn: "HL7 Validator (terminology checking budget)",
		Description: "The include lists more codes than -codesystem-validation-size-limit\n" +
			"allows (1000 by default). Checking a code costs a terminology server\n" +
			"round trip, so past the limit the validator checks *none* of them and\n" +
			"issues this hint instead.\n\n" +
			"This is not a finding about your resource. It says a check did not\n" +
			"run, so a clean result here is not evidence that the codes are valid.",
		HowToFix: "Raise or remove the limit if you want the codes checked:\n\n" +
			"  fhirlint validate ./ig/ --codesystem-size-limit 0   # 0 = no limit\n\n" +
			"Expect it to be slow against a remote terminology server — one round\n" +
			"trip per code. Recording the traffic once with `fhirlint tx warm` and\n" +
			"replaying it with --tx-offline makes repeat runs cheap.",
	},
	"conceptmap_vs_too_many_codes": {
		ID:        "CONCEPTMAP_VS_TOO_MANY_CODES",
		Title:     "Too many codes in a ConceptMap group to check them all",
		DefinedIn: "HL7 Validator (terminology checking budget)",
		Description: "The ConceptMap group maps more codes than\n" +
			"-codesystem-validation-size-limit allows (1000 by default), so the\n" +
			"validator checked none of them against the code system.\n\n" +
			"As with the ValueSet form, this reports a check that did not run\n" +
			"rather than a problem with the mapping.",
		HowToFix: "Raise or remove the limit with --codesystem-size-limit (0 = no\n" +
			"limit), or accept that large ConceptMaps are not code-checked and\n" +
			"verify them another way.",
	},
	"codesystem_cs_supp_too_many_codes": {
		ID:        "CODESYSTEM_CS_SUPP_TOO_MANY_CODES",
		Title:     "Too many codes in a CodeSystem supplement to check them all",
		DefinedIn: "HL7 Validator 6.10.2+ (terminology checking budget)",
		Description: "A CodeSystem supplement adds designations or properties to codes in\n" +
			"another code system. This one carries more codes than\n" +
			"-codesystem-validation-size-limit allows (1000 by default), so none\n" +
			"of them were checked against the code system they supplement.\n\n" +
			"New in validator 6.10.2: supplements were not size-limited before,\n" +
			"so a supplement that used to be checked may stop being checked after\n" +
			"an upgrade.",
		HowToFix: "Raise or remove the limit with --codesystem-size-limit (0 = no\n" +
			"limit) to check the supplement's codes again.",
	},
	"obs-6": {
		ID:        "obs-6",
		Title:     "dataAbsentReason SHALL only be present if value[x] is not present",
		DefinedIn: "FHIR R4 Core (invariant on Observation)",
		Description: "An Observation either has a value or a reason the value is absent — not\n" +
			"both. Providing dataAbsentReason alongside a value[x] is contradictory.",
		HowToFix: "Remove `dataAbsentReason` when a `value[x]` is present, or remove the\n" +
			"value and keep `dataAbsentReason` when the value is genuinely absent.",
	},
	"obs-7": {
		ID:        "obs-7",
		Title:     "component code SHALL NOT duplicate the Observation code with a value",
		DefinedIn: "FHIR R4 Core (invariant on Observation)",
		Description: "If an Observation.component.code is the same as the top-level\n" +
			"Observation.code, the top-level value[x] must not be present — the\n" +
			"measurement belongs in the component to avoid ambiguity.",
		HowToFix: "Remove the top-level `value[x]`, or give the component a distinct\n" +
			"`code` from the Observation's `code`.",
	},
}
