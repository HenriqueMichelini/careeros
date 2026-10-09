export const SUPPORTED_LOCALES = ["en", "pt-BR"] as const

export type Locale = typeof SUPPORTED_LOCALES[number]
export type TranslationValue = string | number

const messages = {
  en: {
    "profile.facts.context": "Context",
    "profile.facts.removeContext": "Remove context",
    "profile.facts.title": "Fact details and context",
    "profile.facts.guide":
      "Existing blocks remain whole. Manual corrections are user authored; changed meaning invalidates old support. Move only into an empty compatible field.",
    "profile.facts.error":
      "This edit was not saved. Check the recovery message or choose an empty compatible destination.",
    "profile.facts.up": "Move up",
    "profile.facts.down": "Move down",
    "profile.facts.legacy_block": "Whole block (not decomposed)",
    "profile.facts.statement": "Statement",
    "profile.facts.existing_profile":
      "Existing Profile; original origin unknown",
    "profile.facts.manual_edit": "Written by you",
    "profile.facts.accepted_proposal":
      "Accepted proposal; original origin unknown",
    "profile.facts.ai_review": "AI review; original origin unknown",
    "profile.facts.unsupported": "No source support",
    "profile.facts.supported": "Approved excerpt support",
    "profile.facts.invalidated": "Previous support invalidated",
    "profile.facts.assertion": "Assertion",
    "profile.facts.intent": "Intent",
    "profile.facts.certainty": "Certainty",
    "profile.facts.precision": "Date precision",
    "profile.facts.temporal": "Original date / duration wording",
    "profile.facts.unknown": "Unknown",
    "profile.facts.affirmed": "Affirmed",
    "profile.facts.negated": "Negated",
    "profile.facts.actual": "Actual",
    "profile.facts.aspiration": "Aspiration",
    "profile.facts.certain": "Certain",
    "profile.facts.uncertain": "Uncertain",
    "profile.facts.exact": "Exact",
    "profile.facts.approximate": "Approximate",
    "profile.facts.role_context": "Role context",
    "profile.facts.project_context": "Project context",
    "profile.facts.period_context": "Period context",
    "profile.facts.none": "None",
    "profile.facts.move": "Move fact to",
    "profile.facts.remove": "Remove fact",
    "profile.facts.active": "Active",
    "cv.density": "Content density",
    "cv.densityHelp":
      "Choose how much evidence to include and how tightly to summarize it. Font size is independent. Every mode targets one A4 page; check actual page fit after accepting.",
    "cv.density.compact": "Compact",
    "cv.density.balanced": "Balanced",
    "cv.density.detailed": "Detailed",
    "cv.density.compactHelp":
      "Strongest evidence, fewer supporting details and tighter summaries.",
    "cv.density.balancedHelp":
      "Representative experience and complementary evidence with concise context.",
    "cv.density.detailedHelp":
      "More useful supporting evidence; may overflow one page and need revision.",
    "cv.densityApplied": "Current saved CV density: {{density}}.",
    "cv.densityNoSnapshot": "Current preview has no generated density.",
    "cv.densityPending":
      "Pending setting: generate and accept a proposal to apply it. Current content and edits stay until you choose replacement.",
    "cv.proposedWording": "Proposed wording",
    "cv.resetToSnapshot": "Reset to saved CV source",
    "cv.generateTitle": "General-purpose CV",
    "cv.generatePolicy":
      "Select relevant experience, impact and complementary qualifications from your Profile. Review the proposed summary and included facts before replacing your CV.",
    "cv.generatePrivacyTitle": "What is sent to OpenAI?",
    "cv.generatePrivacy":
      "Professional skills, tools, experience (roles, employers, dates and supporting details), projects and qualifications. Contact details, career goals, salary, employment status, additional information, credential IDs and links are excluded. Review professional fields for sensitive information first. Generated prose uses the selected CV language; original source facts remain available for review.",
    "cv.generateInsufficient":
      "Add professional skills, an experience description, an achievement or project details in Profile first. Goals and contact information alone cannot establish experience.",
    "cv.generateKey":
      "Set your OpenAI API key above to generate. A rejected key must be updated before trying again.",
    "cv.generatedSnapshot":
      "Editing a saved CV snapshot. Profile changes do not alter it; generate again to review a replacement.",
    "cv.generate": "Generate CV",
    "cv.generating": "Generating…",
    "cv.generateProgress":
      "Selecting and consolidating supported facts. Your current CV stays saved.",
    "cv.generateCancel": "Cancel / discard proposal",
    "cv.generateReview": "Review proposed CV",
    "cv.generateSources": "Inspect {{count}} included source facts",
    "cv.generateReplaceNote":
      "Accepting replaces the CV content and its selection/wording edits. Profile and application drafts stay separate. Check page fit after acceptance and revise any overflow.",
    "cv.generateAccept": "Accept and replace CV",
    "cv.generateErrorInput":
      "The professional Profile is empty, invalid or too large for this request. Review it and generate again deliberately.",
    "cv.generateErrorRate":
      "OpenAI rate limit or credits reached. Your CV is preserved; try again when available.",
    "cv.generateErrorTimeout":
      "Generation timed out. Your CV is preserved; generate again to retry.",
    "cv.generateErrorOutput":
      "The proposed CV failed source validation. Your CV is preserved; generate again to retry.",
    "cv.generateErrorOutage":
      "Generation is unavailable. Your CV is preserved; generate again to retry.",
    "language.label": "Language",
    "language.site": "Site language",
    "language.cv": "CV language",
    "language.english": "English",
    "language.portuguese": "Português",
    "nav.apply": "Apply",
    "nav.profile": "Profile",
    "nav.cv": "CV",
    "nav.results": "Results",
    "common.save": "Save",
    "common.clear": "Clear",
    "common.remove": "Remove",
    "common.copy": "Copy",
    "common.copied": "Copied!",
    "common.copyText": "Copy text",
    "common.sample": "Sample",
    "common.sampleContent": "Sample content",
    "common.present": "Present",
    "common.current": "Current",
    "api.keyConfigured": "API KEY ✓",
    "api.setKey": "! SET API KEY",
    "api.openaiKey": "OpenAI API Key",
    "api.placeholder": "sk-proj-...",
    "home.step": "Step 01 — Opportunity Input",
    "home.titleFind": "Find Your",
    "home.titleNext": "Next Role",
    "home.intro":
      "Paste a job posting URL, full job description, or any text about the opportunity. The AI will analyze it and generate tailored application materials from your profile.",
    "home.jobPosting": "Job Posting or Description",
    "home.jobPostingPlaceholder":
      "Paste the full job description, URL, or any text about the role you're applying to. Include requirements, responsibilities, company info — the more detail, the better the tailoring.",
    "home.characterCount": "{{count}} characters",
    "home.postingLimit": "Job Posting limit: 30,720 UTF-8 bytes.",
    "home.checklist": "Checklist",
    "home.status.pending": "Action needed",
    "home.status.ready": "Ready",
    "home.status.processing": "Processing",
    "home.status.confirmation": "Confirmation needed",
    "home.status.complete": "Complete",
    "home.status.failed": "Failed",
    "home.keyReady":
      "Configured — ready to attempt a request; not yet validated by the provider.",

    "home.apiKey": "API Key",
    "home.configured": "Configured",
    "home.setApiKeyAbove": 'Click "! SET API KEY" above',
    "home.profile": "Profile",
    "home.repositoryReady": "Repository ready",
    "home.goToProfile": "Go to Profile and add your info",
    "home.posting": "Posting",
    "home.pasteJobDetails": "Paste job details on the left",
    "home.generating": "Generating...",
    "home.checkingRequirements": "Checking requirements...",
    "home.checkingRequirementsNote":
      "Looking for job requirements your profile may not mention yet.",
    "home.generateMaterials": "Generate Materials",
    "home.generatingNote":
      "Analyzing opportunity and tailoring your application — this may take 20–40 seconds.",
    "home.output": "Output",
    "home.tailoredResume": "Tailored Résumé",
    "home.coverLetter": "Cover Letter",
    "home.applicationQa": "Application Q&A",
    "home.viewPreviousResults": "View Previous Results →",
    "home.generationFailed":
      "Generation failed. Check your API key and try again.",
    "home.jobErrorCapacity":
      "This job posting exceeds the preparation capacity. Try a smaller portion and submit again. Your text and saved Profile are unchanged.",
    "home.gapErrorInput":
      "The profile or job posting is too large or invalid. Review it and try again.",
    "home.gapErrorKey":
      "The OpenAI API key is missing or invalid. Update your key and try again.",
    "home.gapErrorRateLimit":
      "OpenAI is receiving too many requests. Wait a moment before trying again.",
    "home.gapErrorOutage":
      "The qualification check is temporarily unavailable. Try again shortly.",
    "home.gapErrorTimeout":
      "The qualification check took too long and was stopped. You can retry it.",
    "home.gapErrorInvalidOutput":
      "The qualification check returned an incomplete result. Try again.",
    "home.draftErrorInput":
      "The profile, job posting, or confirmed qualifications are invalid or too large. Review them and try again.",
    "home.draftErrorKey":
      "The OpenAI API key is missing or invalid. Update your key and try again.",
    "home.draftErrorRateLimit":
      "OpenAI is receiving too many requests. Wait a moment before trying again.",
    "home.draftErrorOutage":
      "Application generation is temporarily unavailable. Try again shortly.",
    "home.draftErrorTimeout":
      "Application generation took too long and was stopped. You can retry it.",
    "home.draftErrorInvalidOutput":
      "Application generation returned an incomplete result. Try again.",
    "home.gapPromptEyebrow": "Quick profile check",
    "home.gapPromptTitle": "Did we miss something?",
    "home.gapPromptDescription":
      "I couldn't find these job requirements in your profile. Sometimes we forget to mention things we know. Do any of these sound familiar?",
    "home.gapTypeSkill": "Skill",
    "home.gapTypeExperience": "Experience",
    "home.gapConfirmSkill": "I have this skill",
    "home.gapConfirmExperience": "I have this experience",
    "home.gapExampleLabel": "Where have you used it? (optional)",
    "home.gapExamplePlaceholder":
      "Add a quick example or context to help tailor your application.",
    "home.gapPromptPrivacyNote":
      "Only the items you confirm will be used for this application. Your saved profile won't change.",
    "home.backToPosting": "Back to posting",
    "home.generateWithoutThese": "Generate without these",
    "home.generateWithConfirmed": "Generate with {{count}} selected",
    "landing.howItWorks": "How it works",
    "landing.whatYouGet": "What you get",
    "landing.careerOsHome": "CareerOS home",
    "landing.marketingNavigation": "Marketing navigation",
    "landing.openApp": "Open app",
    "landing.tagline": "A better starting point for every application",
    "landing.titleStop": "Stop starting",
    "landing.titleFromScratch": "from scratch.",
    "landing.intro":
      "CareerOS turns your professional story and any job opportunity into a focused, tailored application — without the blank-page spiral.",
    "landing.tryCareerOs": "Try CareerOS",
    "landing.seeHow": "See how it works ↓",
    "landing.profileOpportunityMove":
      "Your profile · The opportunity · A clear next move",
    "landing.studioLabel": "CareerOS / Application studio",
    "landing.targetOpportunity": "Target opportunity",
    "landing.ready": "Ready",
    "landing.storySignals": "Story signals",
    "landing.found": "{{count}} found",
    "landing.productThinking": "Product thinking",
    "landing.crossFunctionalLeadership": "Cross-functional leadership",
    "landing.customerEmpathy": "Customer empathy",
    "landing.yourApplication": "Your application",
    "landing.builtFrom": "Built from your experience, tuned to the role.",
    "landing.oneSource": "One professional source of truth",
    "landing.threeOutputs": "Three useful application outputs",
    "landing.clearerStep": "A clearer next step, every time",
    "landing.calmWorkflow": "A calmer application workflow",
    "landing.bestApplications": "The best applications sound like",
    "landing.you": "you.",
    "landing.workflowIntro":
      "Your experience is the raw material. CareerOS helps you shape it for the opportunity in front of you.",
    "landing.stepOneTitle": "Build your career story once.",
    "landing.stepOneDescription":
      "Keep your experience, strengths, goals, and projects in one living professional repository.",
    "landing.stepTwoTitle": "Bring the opportunity in.",
    "landing.stepTwoDescription":
      "Paste a job description and CareerOS pulls the signal from the noise so you know what matters.",
    "landing.stepThreeTitle": "Apply as yourself — sharper.",
    "landing.stepThreeDescription":
      "Get a tailored résumé, cover letter, and application answers that connect your story to the role.",
    "landing.lessBusywork": "Less busywork. More signal.",
    "landing.oneProfile": "One profile.",
    "landing.threePrecise": "Three precise outputs.",
    "landing.readyWhen": "Ready when you are",
    "landing.startWithWork":
      "Start with the work you have already done. Let the role guide the emphasis.",
    "landing.nextStarts": "Your next application starts here",
    "landing.makeStory": "Make your story easier to",
    "landing.use": "use.",
    "landing.openCareerOs": "Open CareerOS",
    "landing.footer": "Keep your work close. Take the next step clearly.",
    "repo.sections": "Sections",
    "jobField.typesafeMissing":
      "Add your TypeSafe API key in validation settings before checking this Job Posting.",
    "jobField.disclosure":
      "Validation sends the complete Job Posting to TypeSafe Jev on each check and draft attempt. Accepted text goes to OpenAI for qualification checking or drafting. Your own keys and provider credits are required. The TypeSafe key is remembered in this browser and can be cleared here. TypeSafe documents no input training and US hosting; default retention has not been verified.",
    "jobField.request_information":
      "Please add some responsibilities or requirements so CareerOS can tailor your application to this opportunity.",
    "jobField.request_rephrasing":
      "This wording could be an instruction to override CareerOS, but an actual attempt has not been established. Rewrite the Job Posting without override commands, then submit the revised text for validation.",
    "jobField.reject_attack":
      "An attempt to redirect, manipulate, or override CareerOS was detected. This behavior is prohibited and violates the CareerOS input-use terms. The entire submission was stopped. Remove the instructions and submit job information as data.",
    "jobField.irrelevant":
      "No usable job information was found. Add the role and some responsibilities or requirements. Ordinary unrelated notes can stay around useful job details.",
    "jobField.unusable":
      "This text could not be understood as a Job Posting. Add a readable role with responsibilities or requirements and submit again.",
    "field.settings": "TypeSafe validation settings",
    "field.typesafeKey": "Your TypeSafe API key",
    "field.typesafeMissing":
      "Add your TypeSafe API key in validation settings before reviewing professional information.",
    "field.disclosure":
      "Validation sends the complete submitted text to TypeSafe Jev; accepted text then goes to OpenAI for extraction and comparison. Your own keys and provider credits are required. The TypeSafe key is remembered in this browser and can be cleared here. TypeSafe documents no input training and US hosting; default retention has not been verified.",
    "field.ruleTitle": "CareerOS input-use terms",
    "field.rule":
      "For CareerOS AI input fields, submitting instructions intended to redirect, manipulate, or override CareerOS or its processing models is prohibited. Submit career information as data. Detected attempts stop the entire submission. Uncertain wording requires revision and validation again.",
    "field.reject_attack":
      "An attempt to redirect, manipulate, or override CareerOS was detected. This behavior is prohibited and violates the CareerOS input-use terms. The entire submission was stopped. Remove the instructions and submit career facts as data.",
    "field.request_rephrasing":
      "This wording could be an instruction to override CareerOS, but an actual attempt has not been established. Rewrite the professional description without quoted override commands, then submit the revised text for validation.",
    "field.request_information":
      "Add at least one professional fact, such as a skill you use, a qualification, or work you have done.",
    "field.irrelevant":
      "No usable professional information was found. Add a career fact, such as “I use Java.” Ordinary unrelated notes can stay around useful facts.",
    "field.unusable":
      "This text could not be understood as professional information. Write a readable skill, qualification, or work fact and submit again.",
    "field.failure.key":
      "TypeSafe could not validate this submission with your key. Check your TypeSafe key and access in validation settings, then retry.",
    "field.failure.rate_limit":
      "TypeSafe is rate-limiting validation. Check your credits or wait, then deliberately retry.",
    "field.failure.timeout":
      "Validation timed out. Your text is preserved; retry when ready.",
    "field.failure.outage":
      "Validation is unavailable. Your text is preserved; retry when ready.",
    "field.failure.invalid_output":
      "Validation returned an unusable response. Processing stopped; your text is preserved. Retry when ready.",
    "field.working":
      "Validating professional information and preparing suggestions. Your saved Profile has not changed.",
    "repo.ingestTitle": "Add professional information",
    "repo.ingestLabel": "Your professional information",
    "repo.ingestPlaceholder":
      "Paste CVs, career notes, projects, contact details, education, certifications, languages, skills or goals — messy text is welcome…",
    "repo.ingestGuide":
      "Paste one professional fact or substantial raw information; you do not need to organize or polish it first. Repetitions, mixed formats and disorganized notes are welcome. We extract distinct supported facts, consolidate repeated wording and suggest concise changes in the appropriate Profile sections. Review the source, destination and before/after values, edit or reject proposals, then explicitly apply the ones you approve.",
    "repo.bytes": "bytes",
    "repo.ingestWorking": "Validating and preparing…",
    "repo.ingestReview": "Review suggested changes",
    "repo.ingestDiscard": "Discard",
    "repo.discardTitle": "Discard changes?",
    "repo.discardDescription":
      "Your pasted text and unapplied suggestions will be removed.",
    "repo.keepEditing": "Keep editing",
    "repo.ingestProposals": "Suggested Profile changes",
    "repo.ingestTerminology": "Proposed exact terminology",
    "repo.ingestContinue": "Process next portion",
    "repo.ingestPortionIncomplete":
      "This portion reached a result limit or contained unprocessed claims. Save any reviewed changes, then retry unfinished work or restart with smaller portions.",
    "repo.ingestPortionLimits":
      "Each click processes one bounded portion (up to 30 claims / 60 changes). The complete input is validated each time. Review and save or dismiss these suggestions before continuing. The paste stays only in this session.",
    "repo.ingestPortionProgress": "Source portions processed:",
    "repo.ingestPortionAttempted": "Attempted:",
    "repo.ingestPortionCoverageNote":
      "Source progress does not prove every fact was found. Unresolved claims still need review. Only saved facts are compared with later portions; ambiguous references need clarification.",
    "repo.ingestPortionUnplanned":
      "Some source regions could not be planned within the current limits. Keep the original input and restart with a smaller portion or revise the input.",
    "repo.ingestPortionRegions": "Source byte regions",
    "repo.ingestPortionProcessed": "processed",
    "repo.ingestPortionPending": "unfinished",
    "repo.ingestPortionDismiss":
      "Dismiss remaining suggestions for this portion",
    "repo.ingestPortionRetry": "Retry first unfinished portion",
    "repo.ingestPortionSmaller":
      "Restart with smaller portions (saved facts stay)",
    "repo.ingestReviewNote":
      "Nothing is selected. Review the source and each before/after value, then approve the changes you want.",
    "repo.ingestSource": "Source",
    "repo.clarificationAnswer": "Your answer",
    "repo.clarificationReview": "Review revised proposal",
    "repo.clarificationNote":
      "Answer only what you know. Optional details can remain unknown. Nothing is saved until you approve and apply.",
    "repo.clarificationConflict":
      "Which statement should this proposal use? Explain any correction or leave it unresolved.",
    "repo.clarificationRevised":
      "Revised proposal ready for review. Nothing has been saved.",
    "repo.clarificationFailed":
      "Could not revise this proposal. Your pending proposals are preserved; check your keys or try again deliberately.",
    "repo.clarificationEvidence": "Clarification answer",
    "repo.ingestClarify": "Needs clarification",
    "repo.ingestCoverage": "Claims reviewed",
    "repo.ingestInvalidCount": "Invalid claims",
    "repo.ingestUnplacedCount": "Unplaced changes",
    "repo.ingestCoverageLimit":
      "This ledger accounts for returned claims. It does not prove that every fact in the submitted text was discovered.",
    "repo.ingestCapacityReached":
      "The 30-claim limit was reached. More information may remain unprocessed; review the original text before submitting another portion.",
    "repo.ingestSkippedClaim": "Unprocessed claim",
    "repo.ingestRelatedFact": "Existing Profile fact",
    "repo.ingestFactReference": "Fact ID",
    "repo.ingestRevision": "revision",
    "repo.ingestContextReferences": "Context IDs and revisions",
    "repo.ingestAcceptedSource": "Accepted excerpt",
    "repo.ingestNoAcceptedSource":
      "No accepted excerpt supports this existing fact.",
    "repo.ingestRelatedStatement": "Related submitted statement",
    "repo.ingestOutcome.change": "Proposed change",
    "repo.ingestOutcome.exact_duplicate":
      "Exact duplicate — already represented",
    "repo.ingestOutcome.overlap":
      "Overlapping detail — review all retained information",
    "repo.ingestOutcome.additional_support":
      "Additional supporting evidence — review before saving",
    "repo.ingestOutcome.contradiction":
      "Contradiction — both statements retained for review",
    "repo.ingestOutcome.correction":
      "Correction or supersession candidate — resolution required",
    "repo.ingestOutcome.clarification": "Needs clarification",
    "repo.ingestOutcome.unsupported": "Unsupported claim",
    "repo.ingestOutcome.unresolved": "Processing unresolved",
    "repo.ingestReason.no_validated_disposition":
      "No safe comparison outcome was established. An absent change does not mean this fact is already in your Profile.",
    "repo.ingestReason.support_already_retained":
      "The same assertion and its submitted support are already retained.",
    "repo.ingestReason.source_ambiguity":
      "The submitted wording needs clarification.",
    "repo.ingestReason.verified_exact_alias_or_wording":
      "The wording or a verified exact alias matches an existing fact with compatible meaning and context.",
    "repo.ingestReason.validated_operation":
      "Review the source and proposed change before applying.",
    "repo.ingestReason.identity_not_established":
      "The employer, role, project or period could not be matched safely. Keep the entries distinct until you clarify.",
    "repo.ingestReason.invalid_evidence_update":
      "The additional evidence could not be safely attached to an existing fact.",
    "repo.ingestReason.no_validated_operation":
      "No safe change could be produced for this outcome.",
    "repo.ingestAction.evidence": "Add supporting evidence only",
    "repo.ingestSkipped.claim_id": "Invalid claim identifier.",
    "repo.ingestSkipped.duplicate_claim_id": "Repeated claim identifier.",
    "repo.ingestSkipped.claim_text": "Missing or oversized claim wording.",
    "repo.ingestSkipped.source_size": "Source excerpt exceeded the limit.",
    "repo.ingestSkipped.target_count": "Too many destinations.",
    "repo.ingestSkipped.question_size": "Clarification exceeded the limit.",
    "repo.ingestSkipped.source":
      "The source occurrence could not be validated.",
    "repo.ingestSkipped.meaning_or_support":
      "Invalid meaning or supporting references.",
    "repo.ingestSkipped.support_source":
      "Supporting source or temporal wording could not be validated.",
    "repo.ingestSkipped.target": "Invalid destination.",
    "repo.ingestComparisonDetail":
      "Comparison detail in the submitted text’s language",
    "repo.ingestUnvalidatedWording": "Unvalidated extracted wording",
    "repo.ingestShortened": "shortened",
    "repo.ingestNoValidSource":
      "No exact source excerpt could be validated; review the original text.",
    "repo.ingestReason.change":
      "A change is proposed for explicit review before saving.",
    "repo.ingestReason.exact_duplicate":
      "The same assertion is represented by the referenced fact and compatible context.",
    "repo.ingestReason.overlap":
      "Review the shared wording and distinct information against the referenced facts.",
    "repo.ingestReason.additional_support":
      "This assertion has additional supporting evidence. Accept it explicitly to attach that evidence without changing the wording.",
    "repo.ingestReason.contradiction":
      "These statements conflict. Both remain available for review; no resolution has been selected.",
    "repo.ingestReason.correction":
      "This may correct or replace an existing assertion. Review both statements and their support before deciding.",
    "repo.ingestReason.clarification":
      "More information is needed to compare this statement safely.",
    "repo.ingestReason.unsupported":
      "A supported Profile change could not be established for this statement.",
    "repo.ingestReason.unresolved":
      "Processing did not establish a safe disposition. This does not mean the assertion is already in Profile.",
    "repo.ingestNoChange": "No safe comparison outcome was established.",
    "repo.ingestApproveClaim": "Approve linked changes",
    "repo.ingestRejectClaim": "Reject linked changes",
    "repo.ingestBefore": "Before",
    "repo.ingestAfter": "After / suggested value (editable)",
    "repo.ingestResult": "Resulting field",
    "repo.ingestAction.add": "Add",
    "repo.ingestAction.update": "Update",
    "repo.ingestAction.remove": "Remove",
    "repo.ingestFinding.addition": "New fact",
    "repo.ingestFinding.overlap": "Overlap",
    "repo.ingestFinding.conflict": "Conflict",
    "repo.ingestFinding.in_place": "Existing entry",
    "repo.ingestRemoval": "This removes the field value if confirmed.",
    "repo.ingestApply": "Apply approved changes",
    "repo.ingestCancel": "Cancel and discard",
    "repo.ingestStale":
      "Your Profile changed since these suggestions were made. Review it and generate suggestions again.",
    "repo.ingestInvalidEdit":
      "The selected changes cannot form a valid Profile. Review the edited values and linked entry fields.",
    "repo.ingestErrorPreparation":
      "We couldn't prepare this text. Check its characters or try a smaller portion. Your input and saved Profile are unchanged.",
    "repo.ingestErrorCapacity":
      "This text exceeds the preparation capacity. Try a smaller portion. Your input and saved Profile are unchanged.",
    "repo.ingestErrorInput":
      "Enter up to 30,000 UTF-8 bytes of professional information and check the current Profile fields.",
    "repo.status.unspecified": "Not specified",
    "repo.section.profile": "PROFILE",
    "repo.section.profileDesc": "Edit your professional details by section",
    "repo.contactTitle": "Identity & contact",
    "repo.contactDescription":
      "These details appear in your CV header. Leave any field blank to omit it.",
    "repo.fullName": "Full name",
    "repo.email": "Email",
    "repo.phone": "Phone",
    "repo.contactLocation": "Location",
    "repo.professionalLinks": "Professional links",
    "repo.professionalLinksHint":
      "One link per line. Use [label](site.com) to show a custom label.",
    "repo.section.goals": "Career Goals",
    "repo.section.goalsDesc": "Ambitions, target roles, long-term vision",
    "repo.section.skills": "Skills, tools & tech",
    "repo.section.skillsDesc": "Skills, competencies, tools and technologies",
    "repo.section.competencies": "Competencies",
    "repo.section.competenciesDesc": "Core strengths and soft skills",
    "repo.section.experience": "Experience",
    "repo.section.experienceDesc":
      "Work history, responsibilities, achievements",
    "repo.section.tools": "Tools & Tech",
    "repo.section.toolsDesc": "Software, frameworks, platforms",
    "repo.section.projects": "Projects",
    "repo.section.projectsDesc": "Personal and side projects",
    "repo.section.education": "Education",
    "repo.section.educationDesc": "Degrees and schools",
    "repo.section.certifications": "Certifications",
    "repo.section.certificationsDesc": "Credentials and issuers",
    "repo.section.languages": "Languages",
    "repo.section.languagesDesc": "Languages and proficiency",
    "repo.education.add": "+ Add education",
    "repo.education.empty": "No education entries yet",
    "repo.education.degree": "Degree or qualification",
    "repo.education.institution": "Institution",
    "repo.education.location": "Location",
    "repo.education.graduationDate": "Graduation date",
    "repo.education.details": "Details",
    "repo.certification.add": "+ Add certification",
    "repo.certification.empty": "No certifications yet",
    "repo.certification.name": "Certification name",
    "repo.certification.issuer": "Issuer",
    "repo.certification.date": "Date awarded",
    "repo.certification.credentialId": "Credential ID",
    "repo.certification.url": "Credential URL",
    "repo.language.add": "+ Add language",
    "repo.language.empty": "No languages yet",
    "repo.language.name": "Language",
    "repo.language.proficiency": "Proficiency",
    "repo.qualificationTooLong": "Keep this field under 2 KB.",
    "repo.qualificationLimit": "This section can hold up to 40 entries.",
    "repo.section.compensation": "Compensation",
    "repo.section.compensationDesc": "Current and desired salary",
    "repo.section.other": "Other",
    "repo.section.otherDesc": "Other notes and legacy qualifications",
    "repo.otherLegacyNote":
      "Older notes stay here as written. Add qualifications in their own sections when you want them shown as structured CV entries.",
    "repo.aiReview": "AI Review",
    "repo.aiReviewDescription": "Reviews only {{section}}.",
    "repo.sectionProposal": "Review Profile Proposal",
    "repo.sectionNote":
      "Nothing is saved until you accept. Confirm that every claim is accurate before applying.",
    "repo.sectionBefore": "Current wording",
    "repo.sectionAfter": "Proposed wording",
    "repo.sectionSources": "Original sources",
    "repo.sectionAuthored":
      "Your edited wording will be saved as your own statement; previous excerpt support will be invalidated.",
    "repo.sectionRemove": "I deliberately approve removing this fact.",
    "repo.sectionReject": "Reject proposal",
    "repo.sectionAccept": "Accept and save changes",
    "repo.sectionRefused":
      "The provider declined this rewrite. Your Profile is unchanged.",
    "repo.sectionTruncated":
      "The rewrite was incomplete. Your Profile is unchanged.",
    "repo.reviewing": "Reviewing...",
    "repo.reviewWithAi": "Review with AI",
    "repo.setApiKeyFirst": "Set your API key first.",
    "repo.reviewFailed": "Review failed. Try again.",
    "repo.reviewErrorInput":
      "The profile is too large or contains invalid data. Review it and try again.",
    "repo.reviewErrorKey":
      "The OpenAI API key is missing or invalid. Update your key and try again.",
    "repo.reviewErrorRateLimit":
      "OpenAI is receiving too many requests. Wait a moment and try again.",
    "repo.reviewErrorOutage":
      "The review service is temporarily unavailable. Try again shortly.",
    "repo.reviewErrorTimeout":
      "The review took too long and was stopped. Try again with a smaller profile.",
    "repo.reviewErrorInvalidOutput":
      "The review returned an incomplete result. Your saved profile was not changed.",
    "repo.ingestErrorTruncated":
      "This paste produced more proposed changes than the review could return. Try smaller portions. Your saved profile was not changed.",
    "repo.ingestErrorInvalidOutput":
      "We couldn't safely prepare changes from this text. Try a smaller portion. Your saved profile was not changed.",
    "repo.ingestPartialNotice":
      "Some information could not be verified or placed safely. It was left out of the proposed changes. Review what is shown and add the missing details separately.",
    "repo.ingestUnresolvedClaim":
      "No safe change was produced for this fact. Clarify the source and process it again, or edit your Profile manually.",
    "repo.ingestPreviewField": "Show complete field preview",
    "repo.last": "Last",
    "repo.profileIntro":
      "Add your identity and contact details here, then update your professional information in the other sections.",
    "repo.careerGoals": "Career Goals & Ambitions",
    "repo.careerGoalsPlaceholder":
      "Describe your short and long-term career goals. What roles are you targeting? What industries? What kind of impact do you want to make? Where do you see yourself in 3–5 years?",
    "repo.skillsPlaceholder":
      "List your technical and professional skills. Group them by category if helpful.\n\nExample:\nProgramming Languages: Python, TypeScript, Rust, Go\nFrontend: React, Next.js, Vue, TailwindCSS\nBackend: Node.js, FastAPI, Django, PostgreSQL\nCloud: AWS (EC2, S3, Lambda, RDS), GCP, Docker, Kubernetes\n...",
    "repo.skills": "Skills",
    "repo.competencies": "Core Competencies",
    "repo.competenciesPlaceholder":
      "Describe your core strengths, soft skills, and professional competencies.\n\nExamples: Strategic thinking, cross-functional leadership, agile project management, stakeholder communication, data-driven decision making, mentoring junior engineers, system design, technical writing...",
    "repo.tools": "Tools & Technologies",
    "repo.toolsPlaceholder":
      "List the software, frameworks, platforms, and tools you use.\n\nExamples:\nDevelopment: VS Code, Git, GitHub, Docker, Kubernetes, Terraform\nDatabases: PostgreSQL, MongoDB, Redis, Elasticsearch\nCloud Platforms: AWS, GCP, Azure\nDesign: Figma, Sketch\nProject Management: Jira, Linear, Notion, Confluence\nCommunication: Slack, Zoom\n...",
    "repo.additionalInfo": "Additional Professional Information",
    "repo.additionalInfoPlaceholder":
      "Include any additional information relevant to your professional background:\n\nEducation: degrees, universities, graduation years\nCertifications: AWS Solutions Architect, PMP, CPA, etc.\nLanguages: English (native), Spanish (conversational), etc.\nPublications & talks: conference presentations, articles, papers\nAwards & recognition: industry awards, hackathon wins, etc.\nVolunteer work: relevant volunteer roles or open source contributions\nProfessional memberships: industry associations, boards, etc.\nGeographic preferences: cities, remote/hybrid/on-site preferences\nVisa status & work authorization (if relevant)",
    "repo.position": "Position",
    "repo.positions": "Positions",
    "repo.addPosition": "+ Add Position",
    "repo.noPositions": "No positions added yet",
    "repo.addFirstPosition": "Add your first position",
    "repo.jobTitle": "Job Title",
    "repo.company": "Company",
    "repo.location": "Location",
    "repo.startDate": "Start Date",
    "repo.endDate": "End Date",
    "repo.jobTitlePlaceholder": "Senior Software Engineer",
    "repo.companyPlaceholder": "Acme Corp",
    "repo.locationPlaceholder": "San Francisco, CA / Remote",
    "repo.startDatePlaceholder": "Jan 2021",
    "repo.endDatePlaceholder": "Dec 2023",
    "repo.overview": "Overview / Description",
    "repo.overviewPlaceholder":
      "Brief overview of the role and your scope of work.",
    "repo.responsibilities": "Responsibilities",
    "repo.responsibilitiesPlaceholder":
      "Key responsibilities — use one per line or bullet points.",
    "repo.achievements": "Achievements & Impact",
    "repo.achievementsPlaceholder":
      "Quantified achievements and notable outcomes. Include metrics where possible.",
    "repo.project": "Project",
    "repo.projectName": "Project Name",
    "repo.url": "URL",
    "repo.description": "Description",
    "repo.technologiesUsed": "Technologies Used",
    "repo.highlights": "Highlights & Impact",
    "repo.projectNamePlaceholder": "OpenMetrics Dashboard",
    "repo.urlPlaceholder": "https://github.com/you/project",
    "repo.descriptionPlaceholder":
      "What is this project and why did you build it?",
    "repo.technologiesPlaceholder": "React, TypeScript, PostgreSQL, Docker",
    "repo.highlightsPlaceholder":
      "Key features, technical challenges solved, users reached, or results achieved.",
    "repo.projectsCountSingular": "Project",
    "repo.projectsCountPlural": "Projects",
    "repo.addProject": "+ Add Project",
    "repo.noProjects": "No projects added yet",
    "repo.addFirstProject": "Add your first project",
    "repo.employmentStatus": "Employment Status",
    "repo.status.employedFullTime": "Employed — Full-time",
    "repo.status.employedPartTime": "Employed — Part-time",
    "repo.status.employedContract": "Employed — Contract",
    "repo.status.freelance": "Freelance / Self-employed",
    "repo.status.looking": "Actively looking for work",
    "repo.status.open": "Open to opportunities (not actively searching)",
    "repo.status.unemployed": "Unemployed",
    "repo.status.student": "Student",
    "repo.currentCompensation": "Current Compensation",
    "repo.desiredCompensation": "Desired Compensation",
    "repo.currentCompensationPlaceholder":
      "e.g. $120,000/yr + $20k bonus + equity",
    "repo.desiredCompensationPlaceholder": "e.g. $150,000–$180,000/yr",
    "repo.compensationNote":
      "Include base, bonus, equity, and benefits if relevant.",
    "repo.desiredCompensationNote":
      "Total compensation target, including equity if applicable.",
    "cv.step": "Step 03 — CV Studio",
    "cv.titleYourCv": "Your CV,",
    "cv.titleYourTemplate": "Your Template",
    "cv.intro":
      "Shape the master CV that future application templates can build on. This preview uses your Profile details and sample content where information is missing.",
    "cv.fontSize": "Body font size",
    "cv.documentPreview": "CV document preview",
    "cv.sampleName": "Your name",
    "cv.pageOneEnds": "Page 1 ends",
    "cv.overflowNotice":
      "Content extends beyond the first page. All Profile details remain visible below the page boundary.",
    "cv.curation": "Choose CV content",
    "cv.contact": "Contact details",
    "cv.missingContent": "Missing content",
    "cv.curationIntro":
      "Select Profile facts and adjust CV wording while checking the page preview.",
    "cv.editSummary": "Edit CV summary",
    "cv.editBullet": "Edit CV bullet",
    "cv.summaryGuidance": "{{count}} words · suggested 40–80 words",
    "cv.bulletGuidance": "{{count}} characters · suggested up to 160",
    "cv.lengthSuggestion": "Consider shortening for clarity",
    "cv.resetToProfile": "Use Profile wording",
    "cv.cvOnlyNote":
      "These choices and wording are saved only for this CV. Your full Profile remains available for future applications.",
    "cv.fitAttention":
      "The A4 boundary falls near {{section}}; content extends about {{percent}}% beyond it. Shorten wording or deselect content above it.",
    "cv.header": "header",
    "profile.persistence.storage":
      "Your Profile could not be saved. Free browser storage or allow site storage, download your recovery copy, then reload to retry. Your original Profile is retained; current edits are unsaved.",
    "profile.persistence.validation":
      "This Profile could not be safely loaded or saved. Download the recovery copy before reloading and ask for help recovering the data. The original has been retained.",
    "profile.persistence.stale":
      "Your saved Profile changed in another tab. Current edits are unsaved. Download your recovery copy before reloading the latest Profile and reapplying your edits.",
    "profile.persistence.lock":
      "This browser cannot safely coordinate Profile saves. Download your recovery copy and open CareerOS in a browser with Web Locks support on HTTPS or localhost.",
    "profile.persistence.download": "Download recovery copy",
    "profile.persistence.reload": "Reload saved Profile",
    "profile.persistence.loading": "Loading Profile…",
    "cv.saveError":
      "This browser could not save your CV choices. Your edits will be lost if you reload or leave this page.",
    "cv.exportPdf": "Export A4 PDF",
    "cv.exportOverflow":
      "The selected CV exceeds one A4 page. Shorten the text or deselect content, then export again. Nothing was exported.",
    "cv.sampleCareerLine":
      "Product-minded leader · Building teams and experiences that move business forward",
    "cv.sampleContact":
      "name@email.com  ·  +1 555 010 2024  ·  New York, NY  ·  linkedin.com/in/name",
    "cv.professionalProfile": "Professional Summary",
    "cv.sampleProfile":
      "Strategic professional with a record of turning complex challenges into clear plans, strong partnerships, and measurable results. Known for combining thoughtful leadership with a hands-on approach to delivery.",
    "cv.experience": "Professional Experience",
    "cv.defaultLocation": "New York, NY",
    "cv.defaultDescription":
      "Led cross-functional initiatives, aligning team priorities with customer needs and business outcomes.",
    "cv.sampleJobTitle": "Senior Product Manager · Northstar Labs",
    "cv.sampleExperienceDescription":
      "Lead product strategy and delivery for a platform serving 2M+ users across 12 markets.",
    "cv.sampleAchievementOne":
      "Grew activation by 28% through onboarding research and iterative experimentation.",
    "cv.sampleAchievementTwo":
      "Built a cross-functional roadmap that reduced delivery cycle time by 35%.",
    "cv.sampleSecondJob": "Product Manager · Fieldwork",
    "cv.sampleSecondJobDescription":
      "Launched a new customer insights program and helped grow annual recurring revenue by 18%.",
    "cv.skillsCompetencies": "Technical Skills",
    "cv.sampleSkills":
      "Product strategy · Team leadership · Data analysis · Stakeholder management · Roadmapping",
    "cv.education": "Education",
    "cv.certifications": "Certifications",
    "cv.languages": "Languages",
    "cv.degree": "B.S. Business Administration",
    "cv.university": "University of California, Berkeley · 2018",
    "cv.toolsTechnology": "Tools & Technology",
    "cv.selectedProjects": "Selected Projects",
    "cv.additional": "Additional",
    "cv.languagesCertification":
      "English (Native) · Spanish (Professional)\nCertified Scrum Product Owner",
    "cv.previewSettings": "Preview settings",
    "cv.template": "Template",
    "cv.atsCv": "ATS CV",
    "cv.pageFormat": "Page format",
    "cv.a4": "A4",
    "cv.numberOfPages": "Number of pages",
    "cv.accentColor": "Accent color",
    "cv.red": "Red",
    "cv.profileCoverage": "Profile Coverage",
    "cv.added": "Added",
    "cv.notAdded": "Not added",
    "cv.editProfileNote":
      "Edit your Profile to add your details and replace sample content.",
    "results.noResults": "No results yet",
    "results.nothingGenerated": "Nothing Generated",
    "results.noResultsDescription":
      "Go to Apply, paste a job opportunity, and click Generate Materials.",
    "results.goToApply": "Go to Apply",
    "results.generatedApplication": "Generated Application",
    "results.applicationMaterials": "Application Materials",
    "results.newApplication": "New Application",
    "results.summary": "Summary",
    "results.resume": "Résumé",
    "results.coverLetter": "Cover Letter",
    "results.coverSignatureMissing":
      "Add your full name to Profile, then generate a new draft to include your signature.",
    "results.applicationQa": "Application Q&A",
    "results.role": "Role",
    "results.company": "Company",
    "results.notProvided": "Not provided",
    "results.materials": "Materials",
    "results.materialsList": "Résumé + Cover Letter + Q&A",
    "results.roleSummary": "Role Summary",
    "results.resumeFits": "This draft fits on one A4 page.",
    "results.resumeOverflow":
      "This draft extends beyond the first A4 page. All content remains visible below the boundary.",
    "results.resumeMissingIdentity":
      "Add your name and contact details in Profile to complete the résumé header.",
    "results.savePdf": "Save as PDF",
    "results.pdfHelp":
      "In the print dialog, choose Save as PDF, A4 paper, no margins, and turn off headers and footers.",
    "results.pdfOverflow":
      "This résumé exceeds one A4 page. Generate a shorter draft before saving it as PDF.",
    "results.coverPdfOverflow":
      "This cover letter exceeds one A4 page. Generate a shorter draft before saving it as PDF.",
  },
  "pt-BR": {
    "profile.facts.context": "Contexto",
    "profile.facts.removeContext": "Remover contexto",
    "profile.facts.title": "Detalhes e contexto dos fatos",
    "profile.facts.guide":
      "Blocos existentes permanecem inteiros. Correções manuais são de sua autoria; mudanças de significado invalidam o suporte anterior. Mova apenas para um campo compatível vazio.",
    "profile.facts.error":
      "Esta edição não foi salva. Confira a mensagem de recuperação ou escolha um destino compatível vazio.",
    "profile.facts.up": "Mover para cima",
    "profile.facts.down": "Mover para baixo",
    "profile.facts.legacy_block": "Bloco inteiro (não decomposto)",
    "profile.facts.statement": "Afirmação",
    "profile.facts.existing_profile":
      "Perfil existente; origem original desconhecida",
    "profile.facts.manual_edit": "Escrito por você",
    "profile.facts.accepted_proposal":
      "Proposta aceita; origem original desconhecida",
    "profile.facts.ai_review": "Revisão por IA; origem original desconhecida",
    "profile.facts.unsupported": "Sem suporte de fonte",
    "profile.facts.supported": "Suporte de trecho aprovado",
    "profile.facts.invalidated": "Suporte anterior invalidado",
    "profile.facts.assertion": "Afirmação",
    "profile.facts.intent": "Intenção",
    "profile.facts.certainty": "Certeza",
    "profile.facts.precision": "Precisão da data",
    "profile.facts.temporal": "Texto original da data / duração",
    "profile.facts.unknown": "Desconhecido",
    "profile.facts.affirmed": "Afirmado",
    "profile.facts.negated": "Negado",
    "profile.facts.actual": "Real",
    "profile.facts.aspiration": "Aspiração",
    "profile.facts.certain": "Certo",
    "profile.facts.uncertain": "Incerto",
    "profile.facts.exact": "Exata",
    "profile.facts.approximate": "Aproximada",
    "profile.facts.role_context": "Contexto do cargo",
    "profile.facts.project_context": "Contexto do projeto",
    "profile.facts.period_context": "Contexto do período",
    "profile.facts.none": "Nenhum",
    "profile.facts.move": "Mover fato para",
    "profile.facts.remove": "Remover fato",
    "profile.facts.active": "Ativo",
    "cv.density": "Densidade do conteúdo",
    "cv.densityHelp":
      "Escolha quanto incluir e quanto resumir as evidências. O tamanho da fonte é independente. Todos os modos visam uma página A4; confira o ajuste real depois de aceitar.",
    "cv.density.compact": "Compacto",
    "cv.density.balanced": "Equilibrado",
    "cv.density.detailed": "Detalhado",
    "cv.density.compactHelp":
      "Evidências mais fortes, menos detalhes de apoio e resumos mais enxutos.",
    "cv.density.balancedHelp":
      "Experiência representativa e evidências complementares com contexto conciso.",
    "cv.density.detailedHelp":
      "Mais evidências úteis de apoio; pode exceder uma página e precisar de revisão.",
    "cv.densityApplied": "Densidade do CV salvo: {{density}}.",
    "cv.densityNoSnapshot": "A prévia atual não tem densidade gerada.",
    "cv.densityPending":
      "Configuração pendente: gere e aceite uma proposta para aplicar. Conteúdo e edições atuais ficam até você escolher substituí-los.",
    "cv.proposedWording": "Texto proposto",
    "cv.resetToSnapshot": "Restaurar fonte salva do CV",
    "cv.generateTitle": "Currículo geral",
    "cv.generatePolicy":
      "Selecione experiência relevante, impacto e qualificações complementares do Perfil. Revise o resumo proposto e os fatos incluídos antes de substituir seu CV.",
    "cv.generatePrivacyTitle": "O que é enviado à OpenAI?",
    "cv.generatePrivacy":
      "Habilidades, ferramentas, experiência (cargos, empresas, datas e detalhes), projetos e qualificações profissionais. Contato, objetivos, salário, situação de emprego, informações adicionais, IDs de credenciais e links são excluídos. Revise os campos profissionais para remover dados sensíveis antes. O texto gerado usa o idioma selecionado para o currículo; fatos originais permanecem disponíveis para revisão.",
    "cv.generateInsufficient":
      "Adicione habilidades profissionais, descrição de experiência, conquista ou detalhes de projeto no Perfil. Objetivos e contato não comprovam experiência.",
    "cv.generateKey":
      "Configure sua chave de API OpenAI acima. Atualize uma chave rejeitada antes de tentar novamente.",
    "cv.generatedSnapshot":
      "Editando uma cópia salva do CV. Alterações no Perfil não a modificam; gere novamente para revisar uma substituição.",
    "cv.generate": "Gerar currículo",
    "cv.generating": "Gerando…",
    "cv.generateProgress":
      "Selecionando e consolidando fatos comprovados. Seu CV atual permanece salvo.",
    "cv.generateCancel": "Cancelar / descartar proposta",
    "cv.generateReview": "Revisar CV proposto",
    "cv.generateSources": "Inspecionar {{count}} fatos incluídos",
    "cv.generateReplaceNote":
      "Aceitar substitui o conteúdo do CV e suas edições de seleção/texto. Perfil e candidaturas ficam separados. Confira o ajuste à página depois de aceitar e revise excessos.",
    "cv.generateAccept": "Aceitar e substituir CV",
    "cv.generateErrorInput":
      "O Perfil profissional está vazio, inválido ou grande demais para esta solicitação. Revise e gere novamente.",
    "cv.generateErrorRate":
      "Limite ou créditos da OpenAI atingidos. Seu CV foi preservado; tente quando disponível.",
    "cv.generateErrorTimeout":
      "A geração excedeu o tempo. Seu CV foi preservado; gere novamente para tentar.",
    "cv.generateErrorOutput":
      "O CV proposto falhou na validação das fontes. Seu CV foi preservado; gere novamente para tentar.",
    "cv.generateErrorOutage":
      "A geração está indisponível. Seu CV foi preservado; gere novamente para tentar.",
    "language.label": "Idioma",
    "language.site": "Idioma do site",
    "language.cv": "Idioma do currículo",
    "language.english": "English",
    "language.portuguese": "Português",
    "nav.apply": "Aplicar",
    "nav.profile": "Perfil",
    "nav.cv": "Currículo",
    "nav.results": "Resultados",
    "common.save": "Salvar",
    "common.clear": "Limpar",
    "common.remove": "Remover",
    "common.copy": "Copiar",
    "common.copied": "Copiado!",
    "common.copyText": "Copiar texto",
    "common.sample": "Exemplo",
    "common.sampleContent": "Conteúdo de exemplo",
    "common.present": "Atual",
    "common.current": "Atual",
    "api.keyConfigured": "CHAVE API ✓",
    "api.setKey": "! CONFIGURAR CHAVE API",
    "api.openaiKey": "Chave de API da OpenAI",
    "api.placeholder": "sk-proj-...",
    "home.step": "Etapa 01 — Dados da oportunidade",
    "home.titleFind": "Encontre sua",
    "home.titleNext": "Próxima vaga",
    "home.intro":
      "Cole a URL de uma vaga, a descrição completa ou qualquer texto sobre a oportunidade. A IA fará a análise e criará materiais personalizados a partir do seu perfil.",
    "home.jobPosting": "Vaga ou descrição",
    "home.jobPostingPlaceholder":
      "Cole a descrição completa da vaga, a URL ou qualquer texto sobre a oportunidade. Inclua requisitos, responsabilidades e informações sobre a empresa — quanto mais detalhes, melhor será a personalização.",
    "home.characterCount": "{{count}} caracteres",
    "home.postingLimit": "Limite da vaga: 30.720 bytes UTF-8.",
    "home.checklist": "Checklist",
    "home.status.pending": "Ação necessária",
    "home.status.ready": "Pronto",
    "home.status.processing": "Processando",
    "home.status.confirmation": "Confirmação necessária",
    "home.status.complete": "Concluído",
    "home.status.failed": "Falhou",
    "home.keyReady":
      "Configurada — pronta para tentar uma solicitação; ainda não validada pelo provedor.",

    "home.apiKey": "Chave API",
    "home.configured": "Configurada",
    "home.setApiKeyAbove": 'Clique em "! CONFIGURAR CHAVE API" acima',
    "home.profile": "Perfil",
    "home.repositoryReady": "Repositório pronto",
    "home.goToProfile": "Acesse Perfil e adicione suas informações",
    "home.posting": "Vaga",
    "home.pasteJobDetails": "Cole os detalhes da vaga à esquerda",
    "home.generating": "Gerando...",
    "home.checkingRequirements": "Conferindo requisitos...",
    "home.checkingRequirementsNote":
      "Procurando requisitos da vaga que talvez ainda não estejam no seu perfil.",
    "home.generateMaterials": "Gerar materiais",
    "home.generatingNote":
      "Analisando a oportunidade e personalizando sua candidatura — isso pode levar de 20 a 40 segundos.",
    "home.output": "Saída",
    "home.tailoredResume": "Currículo personalizado",
    "home.coverLetter": "Carta de apresentação",
    "home.applicationQa": "Perguntas e respostas",
    "home.viewPreviousResults": "Ver resultados anteriores →",
    "home.generationFailed":
      "Falha ao gerar. Verifique sua chave API e tente novamente.",
    "home.jobErrorCapacity":
      "Esta vaga excede a capacidade de preparação. Tente um trecho menor e envie novamente. Seu texto e Perfil salvo permanecem inalterados.",
    "home.gapErrorInput":
      "O perfil ou a vaga é muito grande ou contém dados inválidos. Revise e tente novamente.",
    "home.gapErrorKey":
      "A chave da API da OpenAI está ausente ou é inválida. Atualize a chave e tente novamente.",
    "home.gapErrorRateLimit":
      "A OpenAI está recebendo muitas solicitações. Aguarde um momento antes de tentar novamente.",
    "home.gapErrorOutage":
      "A verificação de qualificações está temporariamente indisponível. Tente novamente em instantes.",
    "home.gapErrorTimeout":
      "A verificação demorou demais e foi interrompida. Você pode tentar novamente.",
    "home.gapErrorInvalidOutput":
      "A verificação retornou um resultado incompleto. Tente novamente.",
    "home.draftErrorInput":
      "O perfil, a vaga ou as qualificações confirmadas são inválidos ou muito grandes. Revise e tente novamente.",
    "home.draftErrorKey":
      "A chave da API da OpenAI está ausente ou é inválida. Atualize sua chave e tente novamente.",
    "home.draftErrorRateLimit":
      "A OpenAI está recebendo muitas solicitações. Aguarde um momento antes de tentar novamente.",
    "home.draftErrorOutage":
      "A geração da candidatura está temporariamente indisponível. Tente novamente em instantes.",
    "home.draftErrorTimeout":
      "A geração demorou demais e foi interrompida. Você pode tentar novamente.",
    "home.draftErrorInvalidOutput":
      "A geração retornou um resultado incompleto. Tente novamente.",
    "home.gapPromptEyebrow": "Revisão rápida do perfil",
    "home.gapPromptTitle": "Faltou alguma coisa?",
    "home.gapPromptDescription":
      "Não encontrei estes requisitos da vaga no seu perfil. Às vezes esquecemos de mencionar algo que sabemos. Algum deles parece familiar?",
    "home.gapTypeSkill": "Habilidade",
    "home.gapTypeExperience": "Experiência",
    "home.gapConfirmSkill": "Tenho esta habilidade",
    "home.gapConfirmExperience": "Tenho esta experiência",
    "home.gapExampleLabel": "Onde você usou isso? (opcional)",
    "home.gapExamplePlaceholder":
      "Dê um exemplo rápido ou contexto para personalizar sua candidatura.",
    "home.gapPromptPrivacyNote":
      "Somente os itens confirmados serão usados nesta candidatura. Seu perfil salvo não será alterado.",
    "home.backToPosting": "Voltar à vaga",
    "home.generateWithoutThese": "Gerar sem incluir estes itens",
    "home.generateWithConfirmed": "Gerar com {{count}} selecionado(s)",
    "landing.howItWorks": "Como funciona",
    "landing.whatYouGet": "O que você recebe",
    "landing.careerOsHome": "Página inicial do CareerOS",
    "landing.marketingNavigation": "Navegação institucional",
    "landing.openApp": "Abrir app",
    "landing.tagline": "Um ponto de partida melhor para cada candidatura",
    "landing.titleStop": "Pare de começar",
    "landing.titleFromScratch": "do zero.",
    "landing.intro":
      "O CareerOS transforma sua trajetória profissional e qualquer oportunidade em uma candidatura focada e personalizada — sem a ansiedade da página em branco.",
    "landing.tryCareerOs": "Experimentar o CareerOS",
    "landing.seeHow": "Veja como funciona ↓",
    "landing.profileOpportunityMove":
      "Seu perfil · A oportunidade · Um próximo passo claro",
    "landing.studioLabel": "CareerOS / Estúdio de candidatura",
    "landing.targetOpportunity": "Oportunidade-alvo",
    "landing.ready": "Pronto",
    "landing.storySignals": "Sinais da trajetória",
    "landing.found": "{{count}} encontrados",
    "landing.productThinking": "Pensamento de produto",
    "landing.crossFunctionalLeadership": "Liderança multifuncional",
    "landing.customerEmpathy": "Empatia com o cliente",
    "landing.yourApplication": "Sua candidatura",
    "landing.builtFrom":
      "Criado a partir da sua experiência e ajustado à vaga.",
    "landing.oneSource": "Uma fonte profissional de verdade",
    "landing.threeOutputs": "Três entregas úteis para a candidatura",
    "landing.clearerStep": "Um próximo passo mais claro, sempre",
    "landing.calmWorkflow": "Um fluxo de candidatura mais tranquilo",
    "landing.bestApplications": "As melhores candidaturas soam como",
    "landing.you": "você.",
    "landing.workflowIntro":
      "Sua experiência é a matéria-prima. O CareerOS ajuda você a moldá-la para a oportunidade à sua frente.",
    "landing.stepOneTitle": "Conte sua história profissional uma vez.",
    "landing.stepOneDescription":
      "Mantenha sua experiência, seus pontos fortes, objetivos e projetos em um repositório profissional vivo.",
    "landing.stepTwoTitle": "Traga a oportunidade para dentro.",
    "landing.stepTwoDescription":
      "Cole a descrição da vaga e o CareerOS separa o sinal do ruído para mostrar o que importa.",
    "landing.stepThreeTitle": "Candidate-se como você — com mais precisão.",
    "landing.stepThreeDescription":
      "Receba currículo, carta de apresentação e respostas que conectam sua história à vaga.",
    "landing.lessBusywork": "Menos trabalho repetitivo. Mais sinal.",
    "landing.oneProfile": "Um perfil.",
    "landing.threePrecise": "Três entregas precisas.",
    "landing.readyWhen": "Pronto quando você estiver",
    "landing.startWithWork":
      "Comece pelo trabalho que você já fez. Deixe a vaga orientar a ênfase.",
    "landing.nextStarts": "Sua próxima candidatura começa aqui",
    "landing.makeStory": "Torne sua história mais fácil de",
    "landing.use": "usar.",
    "landing.openCareerOs": "Abrir o CareerOS",
    "landing.footer":
      "Mantenha seu trabalho por perto. Dê o próximo passo com clareza.",
    "repo.sections": "Seções",
    "jobField.typesafeMissing":
      "Adicione sua chave de API TypeSafe nas configurações de validação antes de verificar esta vaga.",
    "jobField.disclosure":
      "A validação envia todo o texto da vaga ao TypeSafe Jev em cada verificação e tentativa de gerar o rascunho. O texto aceito segue para a OpenAI para verificar qualificações ou gerar o rascunho. São necessárias suas próprias chaves e créditos dos provedores. A chave TypeSafe fica salva neste navegador e pode ser apagada aqui. A TypeSafe documenta que não treina com os dados de entrada e que hospeda nos EUA; a retenção padrão não foi verificada.",
    "jobField.request_information":
      "Adicione algumas responsabilidades ou requisitos para que o CareerOS possa adaptar sua candidatura a esta oportunidade.",
    "jobField.request_rephrasing":
      "Esta redação pode ser uma instrução para sobrepor o CareerOS, mas uma tentativa real não foi estabelecida. Reescreva a descrição da vaga sem comandos de sobreposição e envie o texto revisado para validação.",
    "jobField.reject_attack":
      "Foi detectada uma tentativa de redirecionar, manipular ou sobrepor o CareerOS. Esse comportamento é proibido e viola os termos de uso dos campos do CareerOS. Toda a submissão foi interrompida. Remova as instruções e envie informações da vaga como dados.",
    "jobField.irrelevant":
      "Não foram encontradas informações úteis sobre a vaga. Adicione o cargo e algumas responsabilidades ou requisitos. Notas comuns sem relação podem permanecer junto aos detalhes úteis da vaga.",
    "jobField.unusable":
      "Não foi possível entender este texto como uma descrição de vaga. Adicione um cargo legível com responsabilidades ou requisitos e envie novamente.",
    "field.settings": "Configurações de validação TypeSafe",
    "field.typesafeKey": "Sua chave de API TypeSafe",
    "field.typesafeMissing":
      "Adicione sua chave de API TypeSafe nas configurações de validação antes de revisar informações profissionais.",
    "field.disclosure":
      "A validação envia todo o texto submetido ao TypeSafe Jev; o texto aceito segue para a OpenAI para extração e comparação. São necessárias suas próprias chaves e créditos dos provedores. A chave TypeSafe fica salva neste navegador e pode ser apagada aqui. A TypeSafe documenta que não treina com os dados de entrada e que hospeda nos EUA; a retenção padrão não foi verificada.",
    "field.ruleTitle": "Termos de uso de entradas do CareerOS",
    "field.rule":
      "Nos campos de entrada de IA do CareerOS, é proibido enviar instruções destinadas a redirecionar, manipular ou substituir o comportamento do CareerOS ou de seus modelos de processamento. Envie informações profissionais como dados. Tentativas detectadas interrompem toda a submissão. Formulações incertas exigem reformulação e nova validação.",
    "field.reject_attack":
      "Foi detectada uma tentativa de redirecionar, manipular ou substituir o comportamento do CareerOS. Esse comportamento é proibido e viola os termos de uso de entradas do CareerOS. Toda a submissão foi interrompida. Remova as instruções e envie fatos profissionais como dados.",
    "field.request_rephrasing":
      "Esta formulação pode ser uma instrução para substituir o comportamento do CareerOS, mas uma tentativa real não foi estabelecida. Reformule a descrição profissional sem citar comandos de substituição e envie o texto revisado para validação.",
    "field.request_information":
      "Adicione pelo menos um fato profissional, como uma habilidade que utiliza, uma qualificação ou um trabalho que realizou.",
    "field.irrelevant":
      "Não foram encontradas informações profissionais utilizáveis. Adicione um fato profissional, como “Eu uso Java”. Anotações comuns sem relação com o trabalho podem permanecer junto dos fatos úteis.",
    "field.unusable":
      "Não foi possível entender este texto como informação profissional. Escreva uma habilidade, qualificação ou fato de trabalho legível e envie novamente.",
    "field.failure.key":
      "A TypeSafe não conseguiu validar esta submissão com sua chave. Confira sua chave e acesso TypeSafe nas configurações de validação e tente novamente.",
    "field.failure.rate_limit":
      "A TypeSafe está limitando a validação. Confira seus créditos ou aguarde e tente novamente quando desejar.",
    "field.failure.timeout":
      "O tempo de validação se esgotou. Seu texto foi preservado; tente novamente quando desejar.",
    "field.failure.outage":
      "A validação está indisponível. Seu texto foi preservado; tente novamente quando desejar.",
    "field.failure.invalid_output":
      "A validação retornou uma resposta inutilizável. O processamento parou; seu texto foi preservado. Tente novamente quando desejar.",
    "field.working":
      "Validando informações profissionais e preparando sugestões. Seu Perfil salvo não foi alterado.",
    "repo.ingestTitle": "Adicionar informações profissionais",
    "repo.ingestLabel": "Suas informações profissionais",
    "repo.ingestPlaceholder":
      "Cole currículos, anotações, projetos, contatos, formação, certificações, idiomas, habilidades ou objetivos — texto desorganizado é bem-vindo…",
    "repo.ingestGuide":
      "Cole um fato profissional ou informações brutas extensas; não é preciso organizar ou revisar o texto antes. Repetições, formatos misturados e anotações desorganizadas são bem-vindos. Extraímos fatos distintos com suporte no texto, consolidamos repetições e sugerimos alterações objetivas nas seções adequadas do Perfil. Confira fonte, destino e valores antes/depois, edite ou rejeite sugestões e aplique explicitamente as que aprovar.",
    "repo.bytes": "bytes",
    "repo.ingestWorking": "Validando e preparando…",
    "repo.ingestReview": "Revisar alterações sugeridas",
    "repo.ingestDiscard": "Descartar",
    "repo.discardTitle": "Descartar alterações?",
    "repo.discardDescription":
      "O texto colado e as sugestões não aplicadas serão removidos.",
    "repo.keepEditing": "Continuar editando",
    "repo.ingestProposals": "Alterações sugeridas no Perfil",
    "repo.ingestTerminology": "Terminologia exata proposta",
    "repo.ingestContinue": "Processar próxima parte",
    "repo.ingestPortionIncomplete":
      "Esta parte atingiu um limite de resultado ou contém afirmações não processadas. Salve as alterações revisadas e tente novamente o trabalho pendente ou reinicie com partes menores.",
    "repo.ingestPortionLimits":
      "Cada clique processa uma parte limitada (até 30 afirmações / 60 alterações). O texto completo é validado a cada vez. Revise e salve ou dispense as sugestões antes de continuar. O texto fica somente nesta sessão.",
    "repo.ingestPortionProgress": "Partes da fonte processadas:",
    "repo.ingestPortionAttempted": "Tentadas:",
    "repo.ingestPortionCoverageNote":
      "O progresso da fonte não comprova que todos os fatos foram encontrados. Afirmações não resolvidas ainda precisam de revisão. Somente fatos salvos são comparados com partes posteriores; referências ambíguas precisam de esclarecimento.",
    "repo.ingestPortionUnplanned":
      "Algumas regiões da fonte não puderam ser planejadas nos limites atuais. Preserve o texto original e reinicie com partes menores ou revise o texto.",
    "repo.ingestPortionRegions": "Regiões da fonte em bytes",
    "repo.ingestPortionProcessed": "processada",
    "repo.ingestPortionPending": "não concluída",
    "repo.ingestPortionDismiss": "Dispensar sugestões restantes desta parte",
    "repo.ingestPortionRetry":
      "Tentar novamente a primeira parte não concluída",
    "repo.ingestPortionSmaller":
      "Reiniciar com partes menores (fatos salvos permanecem)",
    "repo.ingestReviewNote":
      "Nada está selecionado. Confira a fonte e os valores antes/depois e aprove as alterações desejadas.",
    "repo.ingestSource": "Fonte",
    "repo.clarificationAnswer": "Sua resposta",
    "repo.clarificationReview": "Revisar proposta atualizada",
    "repo.clarificationNote":
      "Responda apenas o que sabe. Detalhes opcionais podem continuar desconhecidos. Nada é salvo até você aprovar e aplicar.",
    "repo.clarificationConflict":
      "Qual declaração esta proposta deve usar? Explique a correção ou mantenha a questão em aberto.",
    "repo.clarificationRevised":
      "Proposta atualizada pronta para revisão. Nada foi salvo.",
    "repo.clarificationFailed":
      "Não foi possível atualizar esta proposta. Suas propostas pendentes foram preservadas; confira as chaves ou tente novamente de forma explícita.",
    "repo.clarificationEvidence": "Resposta de esclarecimento",
    "repo.ingestClarify": "Precisa de esclarecimento",
    "repo.ingestCoverage": "Afirmações revisadas",
    "repo.ingestInvalidCount": "Afirmações inválidas",
    "repo.ingestUnplacedCount": "Alterações não alocadas",
    "repo.ingestCoverageLimit":
      "Este registro contabiliza as afirmações retornadas. Ele não comprova que todos os fatos do texto enviado foram descobertos.",
    "repo.ingestCapacityReached":
      "O limite de 30 afirmações foi atingido. Pode haver informações não processadas; revise o texto original antes de enviar outra parte.",
    "repo.ingestSkippedClaim": "Afirmação não processada",
    "repo.ingestRelatedFact": "Fato existente no Perfil",
    "repo.ingestFactReference": "ID do fato",
    "repo.ingestRevision": "revisão",
    "repo.ingestContextReferences": "IDs e revisões do contexto",
    "repo.ingestAcceptedSource": "Trecho aceito",
    "repo.ingestNoAcceptedSource":
      "Nenhum trecho aceito dá suporte a este fato existente.",
    "repo.ingestRelatedStatement": "Afirmação enviada relacionada",
    "repo.ingestOutcome.change": "Alteração proposta",
    "repo.ingestOutcome.exact_duplicate": "Duplicata exata — já representada",
    "repo.ingestOutcome.overlap":
      "Detalhe sobreposto — revise todas as informações preservadas",
    "repo.ingestOutcome.additional_support":
      "Evidência adicional — revise antes de salvar",
    "repo.ingestOutcome.contradiction":
      "Contradição — ambas as afirmações preservadas para revisão",
    "repo.ingestOutcome.correction":
      "Candidata a correção ou substituição — requer resolução",
    "repo.ingestOutcome.clarification": "Precisa de esclarecimento",
    "repo.ingestOutcome.unsupported": "Afirmação sem suporte",
    "repo.ingestOutcome.unresolved": "Processamento não resolvido",
    "repo.ingestReason.no_validated_disposition":
      "Não foi possível estabelecer uma comparação segura. A ausência de alteração não significa que este fato já esteja no seu Perfil.",
    "repo.ingestReason.support_already_retained":
      "A mesma afirmação e seu suporte enviado já estão preservados.",
    "repo.ingestReason.source_ambiguity":
      "O texto enviado precisa de esclarecimento.",
    "repo.ingestReason.verified_exact_alias_or_wording":
      "O texto ou um alias exato verificado corresponde a um fato existente com significado e contexto compatíveis.",
    "repo.ingestReason.validated_operation":
      "Revise a fonte e a alteração proposta antes de aplicar.",
    "repo.ingestReason.identity_not_established":
      "Não foi possível identificar com segurança a empresa, função, projeto ou período. Mantenha as entradas separadas até esclarecer.",
    "repo.ingestReason.invalid_evidence_update":
      "Não foi possível vincular com segurança a evidência adicional a um fato existente.",
    "repo.ingestReason.no_validated_operation":
      "Não foi possível produzir uma alteração segura para este resultado.",
    "repo.ingestAction.evidence": "Adicionar apenas evidência de suporte",
    "repo.ingestSkipped.claim_id": "Identificador da afirmação inválido.",
    "repo.ingestSkipped.duplicate_claim_id":
      "Identificador da afirmação repetido.",
    "repo.ingestSkipped.claim_text":
      "Texto da afirmação ausente ou muito longo.",
    "repo.ingestSkipped.source_size": "Trecho de origem excedeu o limite.",
    "repo.ingestSkipped.target_count": "Destinos demais.",
    "repo.ingestSkipped.question_size": "Esclarecimento excedeu o limite.",
    "repo.ingestSkipped.source":
      "Não foi possível validar a ocorrência de origem.",
    "repo.ingestSkipped.meaning_or_support":
      "Significado ou referências de suporte inválidos.",
    "repo.ingestSkipped.support_source":
      "Não foi possível validar o suporte ou o texto temporal.",
    "repo.ingestSkipped.target": "Destino inválido.",
    "repo.ingestComparisonDetail":
      "Detalhe da comparação no idioma do texto enviado",
    "repo.ingestUnvalidatedWording": "Texto extraído não validado",
    "repo.ingestShortened": "abreviado",
    "repo.ingestNoValidSource":
      "Não foi possível validar um trecho exato de origem; revise o texto original.",
    "repo.ingestReason.change":
      "Uma alteração foi proposta para revisão explícita antes de salvar.",
    "repo.ingestReason.exact_duplicate":
      "A mesma afirmação está representada pelo fato referenciado e pelo contexto compatível.",
    "repo.ingestReason.overlap":
      "Compare o texto em comum e as informações distintas com os fatos referenciados.",
    "repo.ingestReason.additional_support":
      "Esta afirmação tem evidência adicional. Aceite explicitamente para vincular a evidência sem alterar o texto.",
    "repo.ingestReason.contradiction":
      "Estas afirmações são conflitantes. Ambas continuam disponíveis para revisão; nenhuma resolução foi escolhida.",
    "repo.ingestReason.correction":
      "Esta afirmação pode corrigir ou substituir uma anterior. Revise ambas e seus suportes antes de decidir.",
    "repo.ingestReason.clarification":
      "São necessárias mais informações para comparar esta afirmação com segurança.",
    "repo.ingestReason.unsupported":
      "Não foi possível estabelecer uma alteração com suporte para esta afirmação.",
    "repo.ingestReason.unresolved":
      "O processamento não estabeleceu um resultado seguro. Isso não significa que a afirmação já esteja no Perfil.",
    "repo.ingestNoChange":
      "Nenhum resultado seguro de comparação foi estabelecido.",
    "repo.ingestApproveClaim": "Aprovar alterações ligadas",
    "repo.ingestRejectClaim": "Rejeitar alterações ligadas",
    "repo.ingestBefore": "Antes",
    "repo.ingestAfter": "Depois / valor sugerido (editável)",
    "repo.ingestResult": "Campo resultante",
    "repo.ingestAction.add": "Adicionar",
    "repo.ingestAction.update": "Atualizar",
    "repo.ingestAction.remove": "Remover",
    "repo.ingestFinding.addition": "Fato novo",
    "repo.ingestFinding.overlap": "Sobreposição",
    "repo.ingestFinding.conflict": "Conflito",
    "repo.ingestFinding.in_place": "Entrada existente",
    "repo.ingestRemoval": "Isto remove o valor do campo se confirmado.",
    "repo.ingestApply": "Aplicar alterações aprovadas",
    "repo.ingestCancel": "Cancelar e descartar",
    "repo.ingestStale":
      "Seu Perfil mudou desde que estas sugestões foram feitas. Revise e gere sugestões novamente.",
    "repo.ingestInvalidEdit":
      "As alterações selecionadas não formam um Perfil válido. Revise os valores editados e os campos da entrada.",
    "repo.ingestErrorPreparation":
      "Não foi possível preparar este texto. Confira os caracteres ou tente uma parte menor. Seu texto e Perfil salvo foram preservados.",
    "repo.ingestErrorCapacity":
      "Este texto excede a capacidade de preparação. Tente uma parte menor. Seu texto e Perfil salvo foram preservados.",
    "repo.ingestErrorInput":
      "Informe até 30.000 bytes UTF-8 de dados profissionais e confira os campos atuais do Perfil.",
    "repo.status.unspecified": "Não informado",
    "repo.section.profile": "PERFIL",
    "repo.section.profileDesc": "Edite seus dados profissionais por seção",
    "repo.contactTitle": "Identidade e contato",
    "repo.contactDescription":
      "Estes dados aparecem no cabeçalho do currículo. Deixe um campo vazio para omiti-lo.",
    "repo.fullName": "Nome completo",
    "repo.email": "E-mail",
    "repo.phone": "Telefone",
    "repo.contactLocation": "Localização",
    "repo.professionalLinks": "Links profissionais",
    "repo.professionalLinksHint":
      "Um link por linha. Use [texto](site.com) para mostrar um nome personalizado.",
    "repo.section.goals": "Objetivos de carreira",
    "repo.section.goalsDesc":
      "Ambições, cargos desejados e visão de longo prazo",
    "repo.section.skills": "Habilidades e tecnologias",
    "repo.section.skillsDesc":
      "Habilidades, competências, ferramentas e tecnologias",
    "repo.section.competencies": "Competências",
    "repo.section.competenciesDesc":
      "Pontos fortes e habilidades comportamentais",
    "repo.section.experience": "Experiência",
    "repo.section.experienceDesc":
      "Histórico profissional, responsabilidades e conquistas",
    "repo.section.tools": "Ferramentas e tecnologia",
    "repo.section.toolsDesc": "Softwares, frameworks e plataformas",
    "repo.section.projects": "Projetos",
    "repo.section.projectsDesc": "Projetos pessoais e paralelos",
    "repo.section.education": "Formação",
    "repo.section.educationDesc": "Cursos e instituições",
    "repo.section.certifications": "Certificações",
    "repo.section.certificationsDesc": "Credenciais e emissores",
    "repo.section.languages": "Idiomas",
    "repo.section.languagesDesc": "Idiomas e proficiência",
    "repo.education.add": "+ Adicionar formação",
    "repo.education.empty": "Nenhuma formação cadastrada",
    "repo.education.degree": "Curso ou título",
    "repo.education.institution": "Instituição",
    "repo.education.location": "Localização",
    "repo.education.graduationDate": "Data de conclusão",
    "repo.education.details": "Detalhes",
    "repo.certification.add": "+ Adicionar certificação",
    "repo.certification.empty": "Nenhuma certificação cadastrada",
    "repo.certification.name": "Nome da certificação",
    "repo.certification.issuer": "Emissor",
    "repo.certification.date": "Data de emissão",
    "repo.certification.credentialId": "ID da credencial",
    "repo.certification.url": "URL da credencial",
    "repo.language.add": "+ Adicionar idioma",
    "repo.language.empty": "Nenhum idioma cadastrado",
    "repo.language.name": "Idioma",
    "repo.language.proficiency": "Proficiência",
    "repo.qualificationTooLong": "Mantenha este campo abaixo de 2 KB.",
    "repo.qualificationLimit": "Esta seção comporta até 40 itens.",
    "repo.section.compensation": "Remuneração",
    "repo.section.compensationDesc": "Remuneração atual e desejada",
    "repo.section.other": "Outros",
    "repo.section.otherDesc": "Outras notas e qualificações antigas",
    "repo.otherLegacyNote":
      "Suas notas antigas permanecem aqui como foram escritas. Adicione qualificações nas seções próprias para exibi-las como itens estruturados no currículo.",
    "repo.aiReview": "Revisão por IA",
    "repo.aiReviewDescription": "Revisa apenas {{section}}.",
    "repo.sectionProposal": "Revisar proposta do Perfil",
    "repo.sectionNote":
      "Nada é salvo até você aceitar. Confirme a precisão de cada afirmação antes de aplicar.",
    "repo.sectionBefore": "Texto atual",
    "repo.sectionAfter": "Texto proposto",
    "repo.sectionSources": "Fontes originais",
    "repo.sectionAuthored":
      "O texto editado será salvo como sua própria afirmação; o suporte dos trechos anteriores será invalidado.",
    "repo.sectionRemove": "Aprovo deliberadamente a remoção deste fato.",
    "repo.sectionReject": "Rejeitar proposta",
    "repo.sectionAccept": "Aceitar e salvar alterações",
    "repo.sectionRefused":
      "O provedor recusou a revisão. Seu Perfil foi preservado.",
    "repo.sectionTruncated":
      "A revisão ficou incompleta. Seu Perfil foi preservado.",
    "repo.reviewing": "Revisando...",
    "repo.reviewWithAi": "Revisar com IA",
    "repo.setApiKeyFirst": "Configure sua chave API primeiro.",
    "repo.reviewFailed": "Falha na revisão. Tente novamente.",
    "repo.reviewErrorInput":
      "O perfil é muito grande ou contém dados inválidos. Revise as informações e tente novamente.",
    "repo.reviewErrorKey":
      "A chave da API da OpenAI está ausente ou é inválida. Atualize a chave e tente novamente.",
    "repo.reviewErrorRateLimit":
      "A OpenAI está recebendo muitas solicitações. Aguarde um momento e tente novamente.",
    "repo.reviewErrorOutage":
      "O serviço de revisão está temporariamente indisponível. Tente novamente em instantes.",
    "repo.reviewErrorTimeout":
      "A revisão demorou demais e foi interrompida. Tente novamente com um perfil menor.",
    "repo.reviewErrorInvalidOutput":
      "A revisão retornou um resultado incompleto. Seu perfil salvo não foi alterado.",
    "repo.ingestErrorTruncated":
      "Este texto gerou mais alterações propostas do que a revisão conseguiu retornar. Tente dividir em partes menores. Seu perfil salvo não foi alterado.",
    "repo.ingestErrorInvalidOutput":
      "Não foi possível preparar alterações seguras a partir deste texto. Tente uma parte menor. Seu perfil salvo não foi alterado.",
    "repo.ingestPartialNotice":
      "Algumas informações não puderam ser verificadas ou alocadas com segurança e ficaram fora das alterações propostas. Revise o que aparece e adicione os detalhes ausentes separadamente.",
    "repo.ingestUnresolvedClaim":
      "Não foi possível propor uma alteração segura para este fato. Esclareça a fonte e processe novamente ou edite seu Perfil manualmente.",
    "repo.ingestPreviewField": "Mostrar prévia completa do campo",
    "repo.last": "Última revisão",
    "repo.profileIntro":
      "Adicione seus dados de identidade e contato aqui e atualize suas informações profissionais nas outras seções.",
    "repo.careerGoals": "Objetivos e ambições de carreira",
    "repo.careerGoalsPlaceholder":
      "Descreva seus objetivos de carreira de curto e longo prazo. Quais cargos você busca? Em quais setores? Que impacto deseja gerar? Onde você se vê daqui a 3–5 anos?",
    "repo.skillsPlaceholder":
      "Liste suas habilidades técnicas e profissionais. Se ajudar, agrupe-as por categoria.\n\nExemplo:\nLinguagens: Python, TypeScript, Rust, Go\nFrontend: React, Next.js, Vue, TailwindCSS\nBackend: Node.js, FastAPI, Django, PostgreSQL\nCloud: AWS (EC2, S3, Lambda, RDS), GCP, Docker, Kubernetes\n...",
    "repo.skills": "Habilidades",
    "repo.competencies": "Competências principais",
    "repo.competenciesPlaceholder":
      "Descreva seus principais pontos fortes, habilidades comportamentais e competências profissionais.\n\nExemplos: pensamento estratégico, liderança multifuncional, gestão ágil de projetos, comunicação com stakeholders, tomada de decisão orientada por dados, mentoria de engenheiros juniores, arquitetura de sistemas, redação técnica...",
    "repo.tools": "Ferramentas e tecnologias",
    "repo.toolsPlaceholder":
      "Liste os softwares, frameworks, plataformas e ferramentas que você utiliza.\n\nExemplos:\nDesenvolvimento: VS Code, Git, GitHub, Docker, Kubernetes, Terraform\nBancos de dados: PostgreSQL, MongoDB, Redis, Elasticsearch\nCloud: AWS, GCP, Azure\nDesign: Figma, Sketch\nGestão de projetos: Jira, Linear, Notion, Confluence\nComunicação: Slack, Zoom\n...",
    "repo.additionalInfo": "Informações profissionais adicionais",
    "repo.additionalInfoPlaceholder":
      "Inclua qualquer informação adicional relevante para sua trajetória profissional:\n\nFormação: cursos, universidades e anos de conclusão\nCertificações: AWS Solutions Architect, PMP, CPA etc.\nIdiomas: inglês (nativo), espanhol (conversação) etc.\nPublicações e palestras: apresentações, artigos e trabalhos\nPrêmios e reconhecimentos: prêmios do setor, hackathons etc.\nVoluntariado: funções relevantes ou contribuições open source\nAssociações profissionais: entidades, conselhos etc.\nPreferências geográficas: cidades, remoto/híbrido/presencial\nStatus de visto e autorização de trabalho (se relevante)",
    "repo.position": "Cargo",
    "repo.positions": "Cargos",
    "repo.addPosition": "+ Adicionar cargo",
    "repo.noPositions": "Nenhum cargo adicionado",
    "repo.addFirstPosition": "Adicionar seu primeiro cargo",
    "repo.jobTitle": "Cargo",
    "repo.company": "Empresa",
    "repo.location": "Localização",
    "repo.startDate": "Data de início",
    "repo.endDate": "Data de término",
    "repo.jobTitlePlaceholder": "Engenheiro de software sênior",
    "repo.companyPlaceholder": "Acme Corp",
    "repo.locationPlaceholder": "São Paulo, SP / Remoto",
    "repo.startDatePlaceholder": "jan. 2021",
    "repo.endDatePlaceholder": "dez. 2023",
    "repo.overview": "Visão geral / Descrição",
    "repo.overviewPlaceholder":
      "Breve visão geral do cargo e do seu escopo de trabalho.",
    "repo.responsibilities": "Responsabilidades",
    "repo.responsibilitiesPlaceholder":
      "Principais responsabilidades — use uma por linha ou marcadores.",
    "repo.achievements": "Conquistas e impacto",
    "repo.achievementsPlaceholder":
      "Conquistas quantificadas e resultados relevantes. Inclua métricas quando possível.",
    "repo.project": "Projeto",
    "repo.projectName": "Nome do projeto",
    "repo.url": "URL",
    "repo.description": "Descrição",
    "repo.technologiesUsed": "Tecnologias utilizadas",
    "repo.highlights": "Destaques e impacto",
    "repo.projectNamePlaceholder": "Dashboard OpenMetrics",
    "repo.urlPlaceholder": "https://github.com/voce/projeto",
    "repo.descriptionPlaceholder":
      "O que é este projeto e por que você o criou?",
    "repo.technologiesPlaceholder": "React, TypeScript, PostgreSQL, Docker",
    "repo.highlightsPlaceholder":
      "Principais funcionalidades, desafios técnicos resolvidos, usuários alcançados ou resultados obtidos.",
    "repo.projectsCountSingular": "Projeto",
    "repo.projectsCountPlural": "Projetos",
    "repo.addProject": "+ Adicionar projeto",
    "repo.noProjects": "Nenhum projeto adicionado",
    "repo.addFirstProject": "Adicionar seu primeiro projeto",
    "repo.employmentStatus": "Situação profissional",
    "repo.status.employedFullTime": "Empregado — período integral",
    "repo.status.employedPartTime": "Empregado — meio período",
    "repo.status.employedContract": "Empregado — contrato",
    "repo.status.freelance": "Freelancer / Autônomo",
    "repo.status.looking": "Buscando trabalho ativamente",
    "repo.status.open": "Aberto a oportunidades (sem busca ativa)",
    "repo.status.unemployed": "Desempregado",
    "repo.status.student": "Estudante",
    "repo.currentCompensation": "Remuneração atual",
    "repo.desiredCompensation": "Remuneração desejada",
    "repo.currentCompensationPlaceholder":
      "ex.: R$ 12.000/mês + bônus + participação",
    "repo.desiredCompensationPlaceholder": "ex.: R$ 15.000–R$ 18.000/mês",
    "repo.compensationNote":
      "Inclua salário-base, bônus, participação e benefícios, se relevante.",
    "repo.desiredCompensationNote":
      "Meta de remuneração total, incluindo participação, se aplicável.",
    "cv.step": "Etapa 03 — Estúdio de currículo",
    "cv.titleYourCv": "Seu currículo,",
    "cv.titleYourTemplate": "Seu modelo",
    "cv.intro":
      "Prepare seu currículo-base para candidaturas futuras. Esta prévia usa seu Perfil e inclui exemplos quando faltam dados.",
    "cv.fontSize": "Tamanho da fonte do corpo do CV",
    "cv.documentPreview": "Prévia do currículo",
    "cv.sampleName": "Seu nome",
    "cv.pageOneEnds": "Fim da página 1",
    "cv.overflowNotice":
      "O conteúdo ultrapassa a primeira página. Todos os dados do Perfil continuam visíveis abaixo do limite da página.",
    "cv.curation": "Escolha o conteúdo do currículo",
    "cv.contact": "Dados de contato",
    "cv.missingContent": "Conteúdo ausente",
    "cv.curationIntro":
      "Selecione dados do Perfil e ajuste o texto do currículo acompanhando a prévia da página.",
    "cv.editSummary": "Editar resumo do currículo",
    "cv.editBullet": "Editar tópico do currículo",
    "cv.summaryGuidance": "{{count}} palavras · sugestão: 40–80 palavras",
    "cv.bulletGuidance": "{{count}} caracteres · sugestão: até 160",
    "cv.lengthSuggestion": "Considere encurtar para dar clareza",
    "cv.resetToProfile": "Usar texto do Perfil",
    "cv.cvOnlyNote":
      "Estas escolhas e textos são salvos apenas para este currículo. O Perfil completo continua disponível para candidaturas futuras.",
    "cv.fitAttention":
      "O limite da página A4 fica perto de {{section}}; o conteúdo continua por cerca de {{percent}}% além dele. Encurte o texto ou desmarque conteúdo acima dele.",
    "cv.header": "cabeçalho",
    "profile.persistence.storage":
      "Não foi possível salvar seu Perfil. Libere espaço ou permita o armazenamento do site, baixe a cópia de recuperação e recarregue para tentar novamente. O Perfil original foi mantido; as alterações atuais não foram salvas.",
    "profile.persistence.validation":
      "Não foi possível carregar ou salvar este Perfil com segurança. Baixe a cópia de recuperação antes de recarregar e peça ajuda para recuperar os dados. O original foi mantido.",
    "profile.persistence.stale":
      "Seu Perfil salvo mudou em outra aba. As alterações atuais não foram salvas. Baixe a cópia de recuperação antes de recarregar o Perfil mais recente e reaplicar suas alterações.",
    "profile.persistence.lock":
      "Este navegador não consegue coordenar o salvamento do Perfil com segurança. Baixe a cópia de recuperação e abra o CareerOS em um navegador com suporte a Web Locks, usando HTTPS ou localhost.",
    "profile.persistence.download": "Baixar cópia de recuperação",
    "profile.persistence.reload": "Recarregar Perfil salvo",
    "profile.persistence.loading": "Carregando Perfil…",
    "cv.saveError":
      "O navegador não conseguiu salvar suas escolhas para o currículo. Suas alterações serão perdidas se você recarregar ou sair desta página.",
    "cv.exportPdf": "Exportar PDF A4",
    "cv.exportOverflow":
      "O currículo selecionado excede uma página A4. Encurte o texto ou desmarque conteúdo e tente exportar novamente. Nada foi exportado.",
    "cv.sampleCareerLine":
      "Líder orientado a produto · Construindo equipes e experiências que impulsionam negócios",
    "cv.sampleContact":
      "nome@email.com  ·  +55 11 5555-0101  ·  São Paulo, SP  ·  linkedin.com/in/nome",
    "cv.professionalProfile": "Resumo profissional",
    "cv.sampleProfile":
      "Profissional estratégico com histórico de transformar desafios complexos em planos claros, parcerias sólidas e resultados mensuráveis. Conhecido por combinar liderança cuidadosa com uma abordagem prática de execução.",
    "cv.experience": "Experiência profissional",
    "cv.defaultLocation": "São Paulo, SP",
    "cv.defaultDescription":
      "Liderou iniciativas multifuncionais, alinhando prioridades da equipe às necessidades dos clientes e aos resultados do negócio.",
    "cv.sampleJobTitle": "Gerente de produto sênior · Northstar Labs",
    "cv.sampleExperienceDescription":
      "Liderou a estratégia e a entrega de produto de uma plataforma usada por mais de 2 milhões de pessoas em 12 mercados.",
    "cv.sampleAchievementOne":
      "Aumentou a ativação em 28% por meio de pesquisa de onboarding e experimentação iterativa.",
    "cv.sampleAchievementTwo":
      "Criou um roadmap multifuncional que reduziu o ciclo de entrega em 35%.",
    "cv.sampleSecondJob": "Gerente de produto · Fieldwork",
    "cv.sampleSecondJobDescription":
      "Lançou um novo programa de insights de clientes e ajudou a aumentar a receita recorrente anual em 18%.",
    "cv.skillsCompetencies": "Habilidades técnicas",
    "cv.sampleSkills":
      "Estratégia de produto · Liderança de equipes · Análise de dados · Gestão de stakeholders · Roadmapping",
    "cv.education": "Formação",
    "cv.certifications": "Certificações",
    "cv.languages": "Idiomas",
    "cv.degree": "Bacharelado em Administração",
    "cv.university": "Universidade de São Paulo · 2018",
    "cv.toolsTechnology": "Ferramentas e tecnologia",
    "cv.selectedProjects": "Projetos selecionados",
    "cv.additional": "Informações adicionais",
    "cv.languagesCertification":
      "Inglês (nativo) · Espanhol (profissional)\nProduct Owner certificado",
    "cv.previewSettings": "Ajustes da prévia",
    "cv.template": "Modelo",
    "cv.atsCv": "Currículo ATS",
    "cv.pageFormat": "Formato da página",
    "cv.a4": "A4",
    "cv.numberOfPages": "Número de páginas",
    "cv.accentColor": "Cor de destaque",
    "cv.red": "Vermelho",
    "cv.profileCoverage": "Cobertura do perfil",
    "cv.added": "Adicionado",
    "cv.notAdded": "Não adicionado",
    "cv.editProfileNote":
      "Edite seu Perfil para adicionar seus dados e substituir o conteúdo de exemplo.",
    "results.noResults": "Nenhum resultado ainda",
    "results.nothingGenerated": "Nada gerado",
    "results.noResultsDescription":
      "Acesse Aplicar, cole uma oportunidade e clique em Gerar materiais.",
    "results.goToApply": "Ir para Aplicar",
    "results.generatedApplication": "Candidatura gerada",
    "results.applicationMaterials": "Materiais de candidatura",
    "results.newApplication": "Nova candidatura",
    "results.summary": "Resumo",
    "results.resume": "Currículo",
    "results.coverLetter": "Carta de apresentação",
    "results.coverSignatureMissing":
      "Adicione seu nome completo ao Perfil e gere um novo rascunho para incluir sua assinatura.",
    "results.applicationQa": "Perguntas e respostas",
    "results.role": "Cargo",
    "results.company": "Empresa",
    "results.notProvided": "Não informado",
    "results.materials": "Materiais",
    "results.materialsList": "Currículo + Carta de apresentação + P&R",
    "results.roleSummary": "Resumo da vaga",
    "results.resumeFits": "Este currículo cabe em uma página A4.",
    "results.resumeOverflow":
      "Este currículo ultrapassa a primeira página A4. Todo o conteúdo continua visível abaixo do limite.",
    "results.resumeMissingIdentity":
      "Adicione seu nome e seus dados de contato no Perfil para completar o cabeçalho do currículo.",
    "results.savePdf": "Salvar como PDF",
    "results.pdfHelp":
      "Na janela de impressão, escolha Salvar como PDF, papel A4, sem margens e desative cabeçalhos e rodapés.",
    "results.pdfOverflow":
      "Este currículo excede uma página A4. Gere uma versão mais curta antes de salvar em PDF.",
    "results.coverPdfOverflow":
      "Esta carta de apresentação excede uma página A4. Gere uma versão mais curta antes de salvar em PDF.",
  },
} satisfies Record<Locale, Record<string, string>>

export type TranslationKey = keyof typeof messages.en

export function translate(
  locale: Locale,
  key: TranslationKey,
  values?: Record<string, TranslationValue>,
): string {
  const template = messages[locale][key] || messages.en[key] || key

  return Object.entries(values || {}).reduce(
    (result, [name, value]) => result.split(`{{${name}}}`).join(String(value)),
    template,
  )
}

export function isLocale(value: string | null): value is Locale {
  return value === "en" || value === "pt-BR"
}

export function getInitialLocale(): Locale {
  try {
    const saved = localStorage.getItem("careeros_locale")
    if (isLocale(saved)) return saved
  } catch {
    // Fall back to the browser language when localStorage is unavailable.
  }

  return typeof navigator !== "undefined" &&
    navigator.language.toLowerCase().startsWith("pt")
    ? "pt-BR"
    : "en"
}
