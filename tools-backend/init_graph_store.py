import json
from llama_index.graph_stores.neo4j import Neo4jPropertyGraphStore
from llama_index.core.graph_stores import EntityNode
from llama_index.embeddings.gemini import GeminiEmbedding

arxiv_taxonomy_dict = json.load(open('arxiv_taxonomy_dict.json'))
graph_store = Neo4jPropertyGraphStore(
    url='neo4j+s://28ea85d7.databases.neo4j.io',
    username='neo4j',
    password='nwUFAJm0g3EFbSe6aPoaRj009VvlewHPPupHqu9RUGU',
)
model = GeminiEmbedding(
    api_key="AIzaSyAbXywaltgihJMRGqDXa8tUk8c-Cf1rpCw",
    model_name="models/text-embedding-004",
    embed_batch_size=16
)

# Helper to create entity nodes
def create_entity(label: str, name: str, properties: dict = None) -> EntityNode:
    # Convert properties to strings to ensure serializability
    properties = {k: str(v) for k, v in (properties or {}).items()}
    return EntityNode(label=label, name=name, properties=properties)

def add_categories_to_neo4j():
    nodes = []
    for category in arxiv_taxonomy_dict:
        nodes.append(create_entity('Category', category["name"], {
            "abbrev": category["abbrev"],
            "description": category["description"],
            "embedding": model.get_text_embedding(category["description"])
        }))
    graph_store.upsert_nodes(nodes)

if __name__ == '__main__':
    add_categories_to_neo4j()