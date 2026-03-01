#!/usr/bin/env python3

import argparse
import os
from moviepy import VideoFileClip
from pyannote.audio import Pipeline
import faster_whisper
import yaml
import torch
from pyannote.audio.pipelines.utils.hook import ProgressHook

# 1. Install necessary libraries:
# pip install moviepy pyannote.audio faster_whisper pyyaml torch

# 2. Install ffmpeg:
# sudo apt-get install ffmpeg

# 3. Authenticate with Hugging Face for pyannote.audio
#    Go to https://huggingface.co/pyannote/speaker-diarization-3.1
#    and https://huggingface.co/pyannote/segmentation-3.0
#    Accept the terms and get an access token from https://huggingface.co/settings/tokens
#    Then, from a terminal, run:
#    huggingface-cli login
#    and paste your token.


def extract_audio(video_path, audio_path):
    print(f"Extracting audio from {video_path} to {audio_path}...")
    video_clip = VideoFileClip(video_path)
    audio_clip = video_clip.audio
    if audio_clip is None:
        print("Video has no audio track.")
        video_clip.close()
        return None
    audio_clip.write_audiofile(audio_path, codec='pcm_s16le')
    video_clip.close()
    print("Audio extraction complete.")
    return audio_path


def get_speaker_diarization(audio_path):
    print("Performing speaker diarization...")
    # For GPU, use "cuda"; for CPU, use "cpu"
    device = "cuda" if torch.cuda.is_available() else "cpu"
    print(f"Using device: {device}")

    try:
        # It will download the models automatically on the first run
        diarization_pipeline = Pipeline.from_pretrained(
            "pyannote/speaker-diarization-3.1",
        )
    except Exception as e:
        print("Could not load the diarization pipeline.")
        print("Please make sure you have accepted the user conditions on huggingface.co.")
        print("Visit https://huggingface.co/pyannote/speaker-diarization-3.1 to accept the user conditions.")
        print("Then, from a terminal, run: huggingface-cli login")
        return None

    if diarization_pipeline is None:
        print("Could not load the diarization pipeline.")
        print("Please make sure you have accepted the user conditions on huggingface.co.")
        print("Visit https://huggingface.co/pyannote/speaker-diarization-3.1 to accept the user conditions.")
        print("Then, from a terminal, run: huggingface-cli login")
        return None

    diarization_pipeline = diarization_pipeline.to(torch.device(device))

    with ProgressHook() as hook:
        diarization = diarization_pipeline(audio_path, hook=hook)

    print("Speaker diarization complete.")
    return diarization


def transcribe_audio(audio_path):
    print("Transcribing audio...")
    # For GPU, use device="cuda" and compute_type="float16" for better performance
    device = "cuda" if torch.cuda.is_available() else "cpu"
    compute_type = "float16" if torch.cuda.is_available() else "auto"

    # It will download the model automatically on the first run
    model = faster_whisper.WhisperModel("large-v3", device=device, compute_type=compute_type)
    segments, _ = model.transcribe(audio_path, word_timestamps=True)
    print("Audio transcription complete.")
    return segments


def generate_notebook(video_path, diarization, transcription_segments):
    print("Generating notebook...")
    notebook_scenes = []

    # Create a list of all words with their timestamps
    all_words = []
    if transcription_segments:
        for segment in transcription_segments:
            for word in segment.words:
                all_words.append({'word': word.word, 'start': word.start, 'end': word.end})

    if diarization:
        for speech_turn, track, speaker in diarization.itertracks(yield_label=True):
            scene_conversations = []
            words_in_turn = [word for word in all_words if word['start'] >= speech_turn.start and word['end'] <= speech_turn.end]

            if not words_in_turn:
                continue

            quote = " ".join([word['word'] for word in words_in_turn])

            scene_conversations.append({
                'speaker': speaker,
                'quote': quote,
            })

            notebook_scenes.append({
                'scene': f"Conversation at {speech_turn.start:.2f}s - {speech_turn.end:.2f}s",
                'conversations': scene_conversations,
            })

    notebook_data = [{
        'event': f'Notebook generated from video: {os.path.basename(video_path)}',
        'metadata': {
            'source_video': os.path.basename(video_path),
        },
        'date': '2025-11-08T00:00:00Z', # Placeholder date
        'scenes': notebook_scenes
    }]

    print("Notebook generation complete.")
    return notebook_data

import yt_dlp

def main():
    parser = argparse.ArgumentParser(description="Generate a langner notebook from a video file.")
    parser.add_argument("cookies_file", help="Cookies file.")
    parser.add_argument("youtube_url", help="YouTube URL of the video to process.")
    parser.add_argument("video_path", help="Path to the video file (e.g., mkv, mp4).")
    parser.add_argument("-o", "--output", help="Path to the output YAML file.", default="output.yml")
    args = parser.parse_args()

    video_path = args.video_path
    youtube_url = args.youtube_url
    cookies_file = args.cookies_file
    ydl_opts = {
        'format': 'bestaudio/best',
        'outtmpl': video_path,
        'cookiefile': cookies_file,
        'postprocessors': [{
            'key': 'FFmpegExtractAudio',
            'preferredcodec': 'mp3',
            'preferredquality': '192',
        }],
    }
    with yt_dlp.YoutubeDL(ydl_opts) as ydl:
        ydl.download([youtube_url])

    output_path = args.output

    if not os.path.exists(video_path):
        print(f"Error: Video file not found at {video_path}")
        return

    # Create a temporary audio file path
    base_name = os.path.splitext(os.path.basename(video_path))[0]
    audio_path = f"{base_name}.wav"

    try:
        # 1. Extract audio from video
        audio_path = extract_audio(video_path, audio_path)
        if audio_path is None:
            return

        # 2. Get speaker diarization
        diarization = get_speaker_diarization(audio_path)
        if diarization is None:
            return

        # 3. Transcribe audio
        transcription_segments = transcribe_audio(audio_path)

        # 4. Generate notebook YAML
        notebook = generate_notebook(video_path, diarization, transcription_segments)

        # 5. Write to output file
        with open(output_path, 'w') as f:
            yaml.dump(notebook, f, sort_keys=False, allow_unicode=True)
        print(f"Successfully generated notebook at {output_path}")

    finally:
        # Clean up the temporary audio file
        if audio_path and os.path.exists(audio_path):
            os.remove(audio_path)
            print(f"Removed temporary audio file: {audio_path}")


if __name__ == "__main__":
    main()
