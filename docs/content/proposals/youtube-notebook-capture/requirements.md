---
title: "Product Requirements"
weight: 1
---

# Product Requirements

## Overview

Add a way to capture the transcript of a YouTube video the user is watching and turn it into a Langner story notebook. The resulting notebook lands in the user's configured notebooks directory automatically and is usable by all existing Langner features (Learn, Quiz, PDF export) without any extra steps.

## Problem

### Manual Notebook Creation Is a Barrier

A Langner user who wants to learn vocabulary from a video they watched must transcribe the dialogue by hand, paste it into a file in the right format, and place it in the right directory. This is tedious enough that it discourages users from using video content as a source for notebooks, even when video would be a more engaging source than books or flashcards.

### YouTube Is a Natural Vocabulary Source but Langner Has No Import Path

YouTube hosts a large amount of spoken content — interviews, lectures, podcasts, documentaries, educational channels — that learners can use as raw material for vocabulary study. Users already watch this content. The gap is purely in getting the words out of the video and into a Langner notebook.

### Existing Tooling in the Repo Is Not Usable in the Watching Flow

The repository contains a script that can generate a notebook from a downloaded video file using local transcription models. This requires a local Python environment, downloaded models, hardware suitable for transcription, and the ability to obtain the video file in advance. It is not a flow that a typical user can follow while actually watching a video they are interested in.

## Goals

- Users can turn a YouTube video they are currently viewing into a Langner story notebook in a single gesture.
- The captured notebook is written to the user's configured notebooks directory without manual file handling.
- The captured notebook works end-to-end with every existing Langner feature that accepts a story notebook — it appears in the Learn section, can be viewed in the notebook detail screen, can be quizzed on, and can be exported to PDF — without any special-case code paths.
- The feature is scoped to YouTube as the only supported source in this release.
- The capture flow runs locally; no user content is sent to third-party services as a side effect of capture.

## User Stories

### Capture a Video

As a Langner user, I want to capture a YouTube video I am watching and save it as a Langner story notebook, so that I can study its vocabulary later without hand-transcribing it.

- The user is viewing a YouTube video.
- The user initiates the capture action.
- A Langner story notebook is produced from the video's transcript.
- The notebook lands in the user's configured notebooks directory.
- The user is shown a confirmation that the notebook was saved and where.

### See the Captured Notebook in Learn

As a Langner user, I want the captured notebook to appear in the Learn section alongside my other story notebooks, so that I don't have to configure anything extra to start studying it.

- The captured notebook appears in the notebook list the next time the user opens it.
- The notebook's title reflects the video title.
- The notebook's metadata reflects identifying information about the source (at minimum the channel name and a link back to the video).

### Preserve Video Structure

As a Langner user, I want the captured notebook to reflect the natural structure of the source video, so that I can navigate it the same way I experienced it.

- When the source video has structural markers (such as chapters), the captured notebook uses them as scene boundaries.
- When the source video does not have structural markers, the notebook is organized into scenes by a consistent rule that keeps each scene at a manageable size.
- The transcript content is preserved in the order it appeared in the video.

### Handle Videos Without Transcripts

As a Langner user, I want to be told clearly when a video cannot be captured, so that I understand why and can decide what to do next.

- If the video has no transcript available, the user is shown a clear message explaining that and no notebook is written.
- If the capture fails for any other reason, the user is shown an actionable error message rather than a silent failure.

### Use Captured Notebooks in Quizzes and PDFs

As a Langner user, I want the captured notebook to work with every existing Langner feature, so that video-sourced vocabulary gets the same learning treatment as book-sourced or flashcard-sourced vocabulary.

- The captured notebook can be opened in the notebook detail screen.
- The captured notebook can be exported to PDF using the existing export feature.
- The captured notebook's words, once definitions are added, participate in quizzes with the same spaced repetition treatment as any other story notebook.

## Requirements

### Functional Requirements

- **F1.** The user can initiate capture of a YouTube video they are currently viewing.
- **F2.** The captured transcript is turned into a story notebook that conforms to the existing Langner story notebook format. No new notebook format is introduced.
- **F3.** The captured notebook is written to a location under the user's configured notebooks directory without manual file handling.
- **F4.** The captured notebook includes identifying metadata for the source video: at minimum the video title, the channel name, and a link back to the original video.
- **F5.** The captured notebook reflects the video's structure: when the video has chapters, they become scene boundaries; otherwise the transcript is segmented into scenes by a consistent rule.
- **F6.** If the video has no transcript available, the feature reports this to the user and writes no notebook.
- **F7.** The captured notebook is immediately usable by every existing Langner feature that accepts a story notebook: Learn, notebook detail, quizzes, and PDF export.
- **F8.** The feature is disableable by the user without requiring them to uninstall or reconfigure unrelated parts of Langner.

### Compliance Requirements

These requirements apply regardless of how similar tools or services in the ecosystem operate.

- **C1.** The feature must comply with YouTube's Terms of Service. Any capture approach that conflicts with the Terms of Service is not acceptable, regardless of whether comparable tools exist elsewhere.
- **C2.** Before implementation begins, the relevant sections of YouTube's Terms of Service must be reviewed, and any clauses that constrain the design must be documented and linked from the implementation plan.
- **C3.** The feature must only access video content that the user is already authorized to view through their normal use of YouTube.
- **C4.** The feature must not circumvent any technical protection measure applied to the video, its audio track, or its transcript.
- **C5.** The feature must not download, store, or re-host the video or audio stream of the source video.
- **C6.** Captured notebooks are for the user's personal local use. The feature must not include any capability that facilitates redistribution, republishing, or public sharing of captured content.
- **C7.** If YouTube's Terms of Service change, or YouTube objects to the integration, the feature must be disableable without requiring users to uninstall Langner.
- **C8.** If a future review finds that an aspect of the feature is no longer compliant with YouTube's Terms of Service, that aspect must be removed or revised before the feature continues to be shipped.

### Quality Requirements

- **Q1.** The common case — a YouTube video with a transcript available — must succeed without the user having to configure anything beyond the existing Langner setup.
- **Q2.** Errors must be reported to the user with messages that explain what happened and what the user can do next. Silent failures are not acceptable.
- **Q3.** Captured notebooks must pass the existing Langner story notebook validation rules.
- **Q4.** Capture of a typical video must complete in a timeframe consistent with other interactive Langner actions.
- **Q5.** Capture must not interfere with the user's ability to continue watching the video.

## Out of Scope

- **Other video services.** Only YouTube is in scope for this release. Any other video service would require its own Terms of Service review and its own proposal.
- **Downloading video or audio.** The feature operates on transcripts only; it does not ingest the video or audio stream.
- **Speaker identification.** YouTube transcripts do not label speakers, and the existing Langner story notebook format already accommodates the "no speaker" case. Speaker diarization is not a goal of this feature.
- **Automatic definition or idiom extraction.** Populating the `definitions` section of a captured notebook automatically is a separate concern. If pursued, it will be proposed separately.
- **Editing captured notebooks through a Langner UI.** Captured notebooks are files on disk and are edited the same way other Langner notebooks are edited today.
- **Sharing or syncing captured notebooks.** Captured notebooks remain on the user's local machine, consistent with how other Langner notebooks work.
- **Retroactive or batch capture.** The feature captures the video the user is currently engaging with; it does not crawl or batch-capture videos the user has previously watched.

## Open Questions

- Should captured notebooks be organized into a dedicated subdirectory for video-sourced content, or mixed into the user's existing story notebooks directory?
- Should there be a confirmation step before writing the notebook (preview, rename, edit metadata) or should capture be one action with no intermediate prompts?
- When a video has transcripts in multiple languages, how should the captured transcript language be chosen — always the one the user currently has selected, or an explicit choice?
- What should happen if the user captures the same video twice — overwrite, duplicate with a suffix, or refuse?
