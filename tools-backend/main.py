import arxiv
import json
import requests
from flask import Flask, request, jsonify
from duckduckgo_search import DDGS
from llama_index.core import SimpleDirectoryReader

arxiv_categories = json.load(open('arxiv_taxonomy_dict.json'))
abbrev_to_fullname = {
    item["abbrev"]: item["name"] for item in arxiv_categories
}
graph_store = None
client = arxiv.Client()
app = Flask(__name__)

@app.route('/fetch-duckduckgo', methods=['POST'])
def fetch_duckduckgo():
    try:
        query = request.json['query']
        max_results = request.json['maxResults']
        results = DDGS().text(query, max_results=max_results)
        formatted_results = [{
            'title': result['title'],
            'link': result['href'],
            'snippet': result['body']
        } for result in results]
        return jsonify({'results': formatted_results})
    except Exception as e:
        return jsonify({'error': f'Error fetching data from DuckDuckGo: {e}'}), 500
    
@app.route('/fetch-arxiv', methods=['POST'])
def fetch_arxiv():
    try:
        query = request.json['query']
        max_results = request.json['maxResults']
        search = arxiv.Search(
            query=query, 
            max_results=max_results
        )
        results = client.results(search)
        formatted_results = [{
            'title': result.title,
            'link': result.pdf_url,
            'snippet': result.summary,
            'authors': [author.name for author in result.authors],
            'published': result.published,
            'categories': [abbrev_to_fullname[category] for category in result.categories],
            'doi': result.doi,
        } for result in results]
        return jsonify({'results': formatted_results})
    except Exception as e:
        return jsonify({'error': f'Error fetching data from ArXiv: {e}'}), 500
    
@app.route('/get-chunks', methods=['POST'])
def get_chunks():
    try:
        link = request.json['link']

        # Download the ArXiv paper from the link
        response = requests.get(link)
        with open('/temp/paper.pdf', 'wb') as f:
            f.write(response.content)
        
        # Extract text from the paper
        reader = SimpleDirectoryReader(input_dir="/temp")
        documents = reader.load_data()

        # Get the text chunks from the paper
        chunks = []
        for doc in documents:
            chunk = {
                "text": doc.text,
                "page_number": int(doc.metadata["page_label"]),
            }
            chunks.append(chunk)

        return jsonify({'chunks': chunks})
    except Exception as e:
        return jsonify({'error': f'Error getting chunks: {e}'}), 500

if __name__ == '__main__':
     app.run(debug=True, port=3000)