# Evaluation Mini Tool

A web-based tool for comparing LLM-generated responses with reference data, specifically designed for the TABS project evaluation.

## Features

- Upload reference and multiple response JSONL files
- Compare responses across different models
- Collapsible view by PMID
- Add and view comments for each aspect of the responses
- Comments are stored in a SQLite database for persistence

## Setup

> [!NOTE]  
> You should install UV from Astral to easily setup the environment: https://docs.astral.sh/uv/getting-started/installation/.

1. Make sure you're in the evaluation-mini directory:
```bash
cd source/evaluation/evaluation-mini
```

2. Create and activate a virtual environment (optional but recommended):
```bash
uv venv --python 3.12
source .venv/bin/activate  # On Windows use: .venv\Scripts\activate
```

3. Install the required dependencies:
```bash
uv pip install -r requirements.txt
```

4. Run the Flask application:
```bash
uv run app.py
```

5. Open your browser and navigate to `http://localhost:5000`

## File Format

The application expects JSONL files with the following structure:

```json
{
    "pmid": "39427441",
    "aspect": "ob",
    "summary": "...",
    "sentences": [1, 2],
    "kps": "...",
    "subclaims": "..."
}
```

- Reference file: Use the reference.jsonl from the evaluation directory
- Response files: Use the .jsonl files from the response directory (e.g., "deepseek-v3.jsonl", "gpt-4o.jsonl")

## Usage

1. Click "Choose File" to select your reference JSONL file
2. Click "Choose Files" to select one or more response JSONL files
3. Click "Process Files" to load and compare the data
4. Click on a PMID to expand/collapse the comparison view
5. Add comments in the text areas below each response
6. Comments are automatically saved to the database

## Database

Comments are stored in a SQLite database (`comments.db`) with the following schema:

- `id`: Unique identifier
- `pmid`: The PMID of the article
- `aspect`: The aspect being commented on (summary/kps/subclaims)
- `model`: The name of the model
- `comment`: The comment text 