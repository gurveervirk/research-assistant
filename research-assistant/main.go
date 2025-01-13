package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sort"
	"os"

	"github.com/hypermodeinc/modus/sdk/go/pkg/models"
	"github.com/hypermodeinc/modus/sdk/go/pkg/models/openai"
	"github.com/hypermodeinc/modus/sdk/go/pkg/neo4j"
	h "github.com/hypermodeinc/modus/sdk/go/pkg/http"
	_ "github.com/hypermodeinc/modus/sdk/go"
)

type DDGResult struct {
	Title   string `json:"title"`
	Link    string `json:"link"`
	Snippet string `json:"snippet"`
}

type DDGResponse struct {
	Results []DDGResult `json:"results"`
}

type ArxivResult struct {
	Title      string   `json:"title"`
	Link       string   `json:"link"`
	Snippet    string   `json:"snippet"`
	Authors    []string `json:"authors"`
	Published  string   `json:"published"`
	Categories []string `json:"categories"`
	Doi        string   `json:"doi"`
}

type ArxivResponse struct {
	Results []ArxivResult `json:"results"`
}

type ArxivRequest struct {
	Query      string `json:"query"`
	MaxResults int    `json:"maxResults"`
}

type DDGRequest struct {
	Query      string `json:"query"`
	MaxResults int    `json:"maxResults"`
}

type PaperInsertRequest struct {
	Papers []ArxivResult `json:"papers"`
}

type Category struct {
    Abbrev      string `json:"abbrev"`
    Name        string `json:"name"`
    Description string `json:"description"`
}

func createEntity(label, name string, properties map[string]any) map[string]interface{} {
    return map[string]interface{}{
        "label":      label,
        "name":       name,
        "properties": properties,
    }
}

func SayHello(name *string) string {
	var s string
	if name == nil {
		s = "World"
	} else {
		s = *name
	}

	return fmt.Sprintf("Hello, %s!", s)
}

var selectedProvider = "gemini"
var connectionName = "neo4j"
var systemMessage = `You are a helpful research assistant. You have the capability to engage in normal conversations, but your primary role is to assist with research-related queries.

**Tools at your disposal:**

*   **DuckDuckGo Search:** For general web searches and retrieving relevant snippets.
*   **Arxiv Search:** For finding research papers on Arxiv.org.
*   **Neo4j Database:** For retrieving:
    *   Best categories for a query.
    *   Best papers for a query and category.
    *   Best text chunks from a specific paper.

**Instructions:**

1. **Understand the User's Intent:** Carefully analyze the user's query to determine if it's a general question or a research-specific request.
2. **Choose the Right Tool:**
    *   For general queries, you may use DuckDuckGo to get more context. You may also use Neo4j to retrieve relevant information from the knowledge base, and you may combine results from both sources.
    *   For research-related queries (e.g., finding papers, specific topics), you can use the following strategy:
        *   Start by using DuckDuckGo to gather context and potentially identify relevant keywords.
        *   Use Neo4j's "retrieveBestCategories" function to find the most relevant categories for the query.
        *   Use the identified keywords and categories to search Arxiv for relevant papers.
        *   Present the Arxiv search results to the user.
        *   If the user approves, insert the papers and their chunks into Neo4j using the "InsertPapersIntoNeo4j" function.
3. **Refine and Retrieve Information:**
    *   If the user is looking for specific information within a paper, use Neo4j's "RetrieveBestPapers" and "RetrieveBestChunks" functions to find the most relevant papers and text chunks within those papers.
    *   When presenting information from Neo4j, always mention that it's coming from the knowledge base (Neo4j graph store).
4. **User asks generally, maybe related to research:**
	*   Use DuckDuckGo to get context.
	*   Use Neo4j to retrieve relevant information from the knowledge base.
	*   Use Arxiv to find research papers, using the above strategy.
	*   Combine results from both sources, citing the source of the information in simple terms.
5. **Use Only One Tool at a Time:** You are designed to use only one tool (DuckDuckGo, Arxiv, or Neo4j functions) in each interaction.
6. **Adhere to the User's Intent:** Always ensure that your responses are relevant to the user's query and intent.
7. **Present the Answer:** Once you have gathered the necessary information, present a clear and concise answer to the user.

**Function Call Format:**

When calling functions, use the following format and ensure that the parameters are valid:

*   **SearchDDG(query: string, maxResults: int):**
    *   query: The search query string.
    *   maxResults: The maximum number of results to return (up to 5).

*   **SearchArxiv(query: string, maxResults: int):**
    *   query: The search query string.
    *   maxResults: The maximum number of results to return (up to 5).

*   **retrieveBestCategories(query: string):**
    *   query: The search query string.

*   **RetrieveBestPapers(query: string, category: string):**
    *   query: The search query string.
    *   category: The category to search within.

*   **RetrieveBestChunks(query: string, paperName: string):**
    *   query: The search query string.
    *   paperName: The name of the paper to search within.

*   **InsertPapersIntoNeo4j():**

**Important Notes:**
If you are using any tools / functions, you are to only write the tool call in json format. The tool call should be the only thing in the response, nothing else.
Example Tool Call:
{
	"tool": "SearchDDG",
	"params": {
		"query": "Quantum Computing",
		"maxResults": 5
	}
}

**Example Interaction:**

**User:** Can you find me some recent papers about quantum computing on Arxiv?

**Assistant:** (Calls SearchDDG to get context, then retrieveBestCategories, then SearchArxiv)
(Presents Arxiv results)
I found these papers on Arxiv related to quantum computing. Would you like me to add them to the knowledge base?

**User:** Yes, please.

**Assistant:** (Calls InsertPapersIntoNeo4j)
Okay, I've added the papers and their chunks to the Neo4j database.

**User:** What are some good categories to search for quantum computing?

**Assistant:** (Calls retrieveBestCategories)
Based on your query, the best categories to search for would be [list of categories].

**User:** What are the best papers on quantum computing in the category "cs.QC"?

**Assistant:** (Calls SearchDDG, retrieveBestCategories on "cs.QC", SearchArxiv, RetrieveBestPapers)
The best papers on quantum computing in the category "cs.QC" are [list of papers] (from both Neo4j and Arxiv).

**User:** Can you summarize the paper with id "2301.12345"?

**Assistant:** (Calls RetrieveBestChunks)
Here's a summary of the most relevant chunks from the paper "2301.12345": [summary of chunks]. (Note that this information is from the knowledge base).
`

var messages = []openai.Message {
	openai.NewSystemMessage(systemMessage),
}

var papers = []ArxivResult {}

var providerToModel = map[string]map[string]string{
	"openai": {
		"llm": "openai-llm",
		"embed": "openai-embedding",
	},
	"gemini": {
		"llm": "gemini-llm",
		"embed": "gemini-embedding",
	},
	"hypermode": {
		"llm": "text-generator",
		"embed": "text-embedding",
	},
}

func SelectProvider(provider string) {
	selectedProvider = provider
}

func PromptLLM() (string, error) {
	model, err := models.GetModel[openai.ChatModel](providerToModel[selectedProvider]["llm"])
	if err != nil {
		return "", fmt.Errorf("failed to get openai chat model: %w", err)
	}
	input, err := model.CreateInput(messages...)
	if err != nil {
		return "", fmt.Errorf("failed to create input for chat model: %w", err)
	}
	output, err := model.Invoke(input)
	if err != nil {
		return "", fmt.Errorf("failed to invoke chat model: %w", err)
	}

	if len(output.Choices) == 0 {
		return "", fmt.Errorf("expected at least one choice, got none")
	}
	return strings.TrimSpace(output.Choices[0].Message.Content), nil
}

func PromptEmbedModel(text string) ([]float32, error) {
	model, err := models.GetModel[openai.EmbeddingsModel](providerToModel[selectedProvider]["embed"])
	if err != nil {
		return nil, fmt.Errorf("failed to get openai embeddings model: %w", err)
	}

	input, err := model.CreateInput(text)
	if err != nil {
		return nil, fmt.Errorf("failed to create input for embeddings model: %w", err)
	}
	output, err := model.Invoke(input)
	if err != nil {
		return nil, fmt.Errorf("failed to invoke embeddings model: %w", err)
	}
	if len(output.Data) == 0 {
		return nil, fmt.Errorf("expected at least one output in embeddings, got none")
	}
	return output.Data[0].Embedding, nil
}

func SearchDDG(query string, maxResults int) (*DDGResponse, error) {
	requestBody, err := json.Marshal(DDGRequest{Query: query, MaxResults: maxResults})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body for ddg: %w", err)
	}
	req := h.NewRequest("http://localhost:3000/fetch-duckduckgo")
	req.Method = "POST"
	req.Body = requestBody
	req.Headers = h.NewHeaders(map[string][]string{
		"Content-Type": {"application/json"},
	})
	resp, err := h.Fetch(req)
	if err != nil {
		return nil, fmt.Errorf("http fetch failed: %w", err)
	}
	if !resp.Ok() {
		body, _ := io.ReadAll(bytes.NewReader(resp.Body))
		return nil, fmt.Errorf("http fetch failed with status %d : %s, body: %s", resp.Status, resp.StatusText, string(body))
	}
	var responseJson DDGResponse
	if err := json.NewDecoder(bytes.NewReader(resp.Body)).Decode(&responseJson); err != nil {
		return nil, fmt.Errorf("failed to decode json response body: %w", err)
	}
	return &responseJson, nil
}

func SearchArxiv(query string, maxResults int) (*ArxivResponse, error) {
	requestBody, err := json.Marshal(ArxivRequest{Query: query, MaxResults: maxResults})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body for arxiv : %w", err)
	}
	req := h.NewRequest("http://localhost:3000/fetch-arxiv")
	req.Method = "POST"
	req.Body = requestBody
	req.Headers = h.NewHeaders(map[string][]string{
		"Content-Type": {"application/json"},
	})
	resp, err := h.Fetch(req)
	if err != nil {
		return nil, fmt.Errorf("http fetch failed: %w", err)
	}
	if !resp.Ok() {
		body, _ := io.ReadAll(bytes.NewReader(resp.Body))
		return nil, fmt.Errorf("http fetch failed with status %d : %s, body: %s", resp.Status, resp.StatusText, string(body))
	}

	var responseJson ArxivResponse
	if err := json.NewDecoder(bytes.NewReader(resp.Body)).Decode(&responseJson); err != nil {
		return nil, fmt.Errorf("failed to decode json response body: %w", err)
	}
	papers = responseJson.Results
	return &responseJson, nil
}

func indexExists(connectionName, indexName string) (bool, error) {
	resp, err := neo4j.ExecuteQuery(
		connectionName,
		"SHOW INDEXES WHERE name = $indexName",
		map[string]any{
			"indexName": indexName,
		},
	)
	if err != nil {
		return false, fmt.Errorf("failed to check if index %s exists: %w", indexName, err)
	}
	return len(resp.Records) > 0, nil
}

func AddCategoriesToNeo4j() error {
    data, err := os.ReadFile("../arxiv_taxonomy_dict.json")
    if err != nil {
        return fmt.Errorf("failed to read arxiv_taxonomy_dict.json: %w", err)
    }

    var categories []Category
    if err := json.Unmarshal(data, &categories); err != nil {
        return fmt.Errorf("failed to unmarshal categories: %w", err)
    }

    nodes := []map[string]interface{}{}
    for _, category := range categories {
		embedding, err := PromptEmbedModel(category.Description)
		if err != nil {
			return fmt.Errorf("failed to embed category description: %w", err)
		}
        properties := map[string]any{
            "abbrev":      category.Abbrev,
            "description": category.Description,
			"embedding": embedding,
        }
        nodes = append(nodes, createEntity("Category", category.Name, properties))
    }

    for _, node := range nodes {
        _, err := neo4j.ExecuteQuery(
            connectionName,
            `
            MERGE (n:Category {name: $name})
            SET n += $properties
            `,
            map[string]interface{}{
                "name":       node["name"],
                "properties": node["properties"],
            },
        )
        if err != nil {
            return fmt.Errorf("failed to upsert node: %w", err)
        }
    }

    return nil
}

func InitializeNeo4j() error {
	// Check if either Category FTS or Vector index exists
	ftsExists, err := indexExists(connectionName, "categoryFTS")
	if err != nil {
		return err
	}
	vectorExists, err := indexExists(connectionName, "categoryVectorIndex")
	if err != nil {
		return err
	}

	if ftsExists || vectorExists {
		fmt.Println("Skipping creation of indexes, already exist")
		return nil
	}

	// Add categories to Neo4j
	if err := AddCategoriesToNeo4j(); err != nil {
		return fmt.Errorf("failed to add categories to neo4j: %w", err)
	}

	// Create a global FTS index on __Entity__ for properties: snippet, description, name, text
	_, err = neo4j.ExecuteQuery(
		connectionName,
		`
		CREATE FULLTEXT INDEX globalFTS IF NOT EXISTS FOR (n:__Entity__) ON EACH [n.snippet, n.description, n.name, n.text]
		`,
		map[string]any{},
	)
	if err != nil {
		return fmt.Errorf("failed to create global FTS index: %w", err)
	}

	// Create FTS index over Category label for description and name
	_, err = neo4j.ExecuteQuery(
		connectionName,
		`
		CREATE FULLTEXT INDEX categoryFTS IF NOT EXISTS FOR (n:Category) ON EACH [n.description, n.name]
		`,
		map[string]any{},
	)
	if err != nil {
		return fmt.Errorf("failed to create FTS index on Category: %w", err)
	}

	// Create Vector index over Category label for embedding
	_, err = neo4j.ExecuteQuery(
		connectionName,
		`
		CREATE VECTOR INDEX categoryVectorIndex IF NOT EXISTS FOR (n:Category) ON (n.embedding) OPTIONS { indexConfig: {  `+"`vector.dimensions`: 768, `vector.similarity_function`: 'cosine' } }"+`
		`,
		map[string]any{},
	)
	if err != nil {
		return fmt.Errorf("failed to create vector index on Category: %w", err)
	}

	// Get unique node names under Category label
	uniqueNamesResp, err := neo4j.ExecuteQuery(
		connectionName,
		`
		MATCH (c:Category)
		RETURN DISTINCT c.name AS uniqueName
		`,
		map[string]any{},
	)
	if err != nil {
		return fmt.Errorf("failed to get unique node names from Category: %w", err)
	}

	// Create FTS and Vector indexes for each unique node name
	for _, record := range uniqueNamesResp.Records {
		uniqueNameResult, ok := record.Get("uniqueName")
		if !ok {
			fmt.Println("failed to get uniqueName from record")
			continue
		}

		// Escape node name with backticks
		escapedUniqueName := fmt.Sprintf("`%s`", uniqueNameResult)

		// Create FTS index for each unique node name over snippet and name
		ftsIndexName := fmt.Sprintf("`%sFTS`", uniqueNameResult)
		_, err = neo4j.ExecuteQuery(
			connectionName,
			fmt.Sprintf(
				`
				CREATE FULLTEXT INDEX %s IF NOT EXISTS FOR (n:%s) ON EACH [n.snippet, n.name]
				`,
				ftsIndexName,
				escapedUniqueName,
			),
			map[string]any{},
		)
		if err != nil {
			return fmt.Errorf("failed to create FTS index on %s: %w", uniqueNameResult, err)
		}

		// Create Vector index for each unique node name over embedding
		vectorIndexName := fmt.Sprintf("`%sVectorIndex`", uniqueNameResult)
		_, err = neo4j.ExecuteQuery(
			connectionName,
			fmt.Sprintf(
				`
				CREATE VECTOR INDEX %s IF NOT EXISTS FOR (n:%s) ON (n.embedding) OPTIONS { indexConfig: { `+"`vector.dimensions`: 768, `vector.similarity_function`: 'cosine' } }"+`
				`,
				vectorIndexName,
				escapedUniqueName,
			),
			map[string]any{},
		)
		if err != nil {
			return fmt.Errorf("failed to create vector index on %s: %w", uniqueNameResult, err)
		}
	}

	// Create FTS index over Author label for name
	exists, err := indexExists(connectionName, "authorFTS")
	if err != nil {
		return err
	}
	if !exists {
		_, err = neo4j.ExecuteQuery(
			connectionName,
			`
			CREATE FULLTEXT INDEX authorFTS IF NOT EXISTS FOR (a:Author) ON EACH [a.name]
			`,
			map[string]any{},
		)
		if err != nil {
			return fmt.Errorf("failed to create FTS index on Author: %w", err)
		}
	} else {
		fmt.Println("Skipping creation of authorFTS index, as it already exists")
	}

	return nil
}

func normalizeScores(results []map[string]any) ([]map[string]any, error) {
	if len(results) == 0 {
		return results, nil
	}

	// Find min and max scores for each type
	minScores := make(map[string]float64)
	maxScores := make(map[string]float64)
	for _, result := range results {
		score, ok := result["score"].(float64)
		if !ok {
			return nil, fmt.Errorf("failed to cast score to float")
		}
		resultType, ok := result["type"].(string)
		if !ok {
			return nil, fmt.Errorf("failed to cast result type to string")
		}

		if _, ok := minScores[resultType]; !ok {
			minScores[resultType] = score
			maxScores[resultType] = score
		} else {
			if score < minScores[resultType] {
				minScores[resultType] = score
			}
			if score > maxScores[resultType] {
				maxScores[resultType] = score
			}
		}
	}

	// Normalize scores
	for _, result := range results {
		resultType := result["type"].(string)
		score, _ := result["score"].(float64)
		minScore := minScores[resultType]
		maxScore := maxScores[resultType]

		if maxScore-minScore != 0 { // Avoid division by zero
			result["score"] = (score - minScore) / (maxScore - minScore)
		} else {
			result["score"] = 0.0 // Or handle it in a way that makes sense for your application
		}
	}

	return results, nil
}

// Function to retrieve best categories based on query
func RetrieveBestCategories(query string) ([]string, error) {
	// Embed the query
	embedding, err := PromptEmbedModel(query)
	if err != nil {
		return nil, fmt.Errorf("failed to embed query: %w", err)
	}
	// Use vector search to find the best matching categories
	vectorResp, err := neo4j.ExecuteQuery(
		"neo4j",
		`
		CALL db.index.vector.queryNodes('categoryVectorIndex', 5, $embedding)
		YIELD node, score
		RETURN node.name AS category, score
		`,
		map[string]any{
			"embedding": embedding,
		},

	)
	if err != nil {
		return nil, fmt.Errorf("failed to query category vector index: %w", err)
	}

	// Use FTS to find the best matching categories based on the query
	ftsResp, err := neo4j.ExecuteQuery(
		"neo4j",
		`
		CALL db.index.fulltext.queryNodes('categoryFTS', $query)
		YIELD node, score
		RETURN node.name AS category, score
		`,
		map[string]any{
			"query": query,
		},

	)
	if err != nil {
		return nil, fmt.Errorf("failed to query category FTS index: %w", err)
	}

	// Combine the results
	var combinedResults []map[string]any
	for _, record := range vectorResp.Records {
		category, ok := record.Get("category")
		score, ok2 := record.Get("score")
		if ok && ok2 {
			combinedResults = append(combinedResults, map[string]any{"category": category, "score": score, "type": "vector"})
		}
	}
	for _, record := range ftsResp.Records {
		category, ok := record.Get("category")
		score, ok2 := record.Get("score")
		if ok && ok2 {
			combinedResults = append(combinedResults, map[string]any{"category": category, "score": score, "type": "fts"})
		}
	}

	// Normalize scores
	combinedResults, err = normalizeScores(combinedResults)
	if err != nil {
		return nil, fmt.Errorf("failed to normalize scores: %w", err)
	}

	// Sort combined results by score in descending order
	sort.Slice(combinedResults, func(i, j int) bool {
		return combinedResults[i]["score"].(float64) > combinedResults[j]["score"].(float64)
	})

	// Get top 5 categories
	var categories []string
	for i := 0; i < len(combinedResults) && i < 5; i++ {
		categories = append(categories, combinedResults[i]["category"].(string))
	}

	return categories, nil
}

// Function to retrieve best papers based on query and category
func RetrieveBestPapers(query string, category string) ([]string, error) {
	// Embed the query
	embedding, err := PromptEmbedModel(query)
	if err != nil {
		return nil, fmt.Errorf("failed to embed query: %w", err)
	}
	// Use a combination of FTS and vector search to find the best papers
	ftsResp, err := neo4j.ExecuteQuery(
		"neo4j",
		fmt.Sprintf(
			`
			CALL db.index.fulltext.queryNodes('%sFTS', $query)
			YIELD node, score
			RETURN node.name AS paper, score, 'fts' AS type
			`,
			category,
		),
		map[string]any{
			"query": query,
		},

	)
	if err != nil {
		return nil, fmt.Errorf("failed to query %s FTS index: %w", category, err)
	}

	vectorResp, err := neo4j.ExecuteQuery(
		"neo4j",
		fmt.Sprintf(
			`
			CALL db.index.vector.queryNodes('%sVectorIndex', 5, $embedding)
			YIELD node, score
			RETURN node.name AS paper, score, 'vector' AS type
			`,
			category,
		),
		map[string]any{
			"embedding": embedding,
		},

	)
	if err != nil {
		return nil, fmt.Errorf("failed to query %s vector index: %w", category, err)
	}

	// Combine the results
	var combinedResults []map[string]any
	for _, record := range ftsResp.Records {
		paper, ok := record.Get("paper")
		score, ok2 := record.Get("score")
		if ok && ok2 {
			combinedResults = append(combinedResults, map[string]any{"paper": paper, "score": score, "type": "fts"})
		}
	}
	for _, record := range vectorResp.Records {
		paper, ok := record.Get("paper")
		score, ok2 := record.Get("score")
		if ok && ok2 {
			combinedResults = append(combinedResults, map[string]any{"paper": paper, "score": score, "type": "vector"})
		}
	}

	// Normalize scores
	combinedResults, err = normalizeScores(combinedResults)
	if err != nil {
		return nil, fmt.Errorf("failed to normalize scores: %w", err)
	}

	// Sort combined results by score in descending order
	sort.Slice(combinedResults, func(i, j int) bool {
		return combinedResults[i]["score"].(float64) > combinedResults[j]["score"].(float64)
	})

	// Get top 5 papers
	var papers []string
	for i := 0; i < len(combinedResults) && i < 5; i++ {
		papers = append(papers, combinedResults[i]["paper"].(string))
	}

	return papers, nil
}

// Function to retrieve best chunks based on query and paper name
func RetrieveBestChunks(query string, paperName string) ([]string, error) {
	// Embed the query
	embedding, err := PromptEmbedModel(query)
	if err != nil {
		return nil, fmt.Errorf("failed to embed query: %w", err)
	}
	// Use FTS to find relevant chunks based on the query
	ftsResp, err := neo4j.ExecuteQuery(
		"neo4j",
		fmt.Sprintf(
			`
			CALL db.index.fulltext.queryNodes('globalFTS', $query)
			YIELD node, score
			WHERE node:"%s"
			RETURN node.text AS chunk, score, 'fts' AS type
			`,
			paperName,
		),
		map[string]any{
			"query":     query,
			"paperName": paperName,
		},

	)
	if err != nil {
		return nil, fmt.Errorf("failed to query globalFTS for paper %s: %w", paperName, err)
	}

	// Use vector search to find the best matching chunks within a paper
	vectorResp, err := neo4j.ExecuteQuery(
		"neo4j",
		fmt.Sprintf(
			`
			CALL db.index.vector.queryNodes('%sVectorIndex', 5, $embedding)
			YIELD node, score
			WHERE node:"$paperName"
			RETURN node.text AS chunk, score, 'vector' AS type
			`,
			paperName,
		),
		map[string]any{
			"paperName": paperName,
			"embedding": embedding,
		},

	)
	if err != nil {
		return nil, fmt.Errorf("failed to query %s vector index: %w", paperName, err)
	}

	// Combine the results
	var combinedResults []map[string]any
	for _, record := range ftsResp.Records {
		chunk, ok := record.Get("chunk")
		score, ok2 := record.Get("score")
		if ok && ok2 {
			combinedResults = append(combinedResults, map[string]any{"chunk": chunk, "score": score, "type": "fts"})
		}
	}
	for _, record := range vectorResp.Records {
		chunk, ok := record.Get("chunk")
		score, ok2 := record.Get("score")
		if ok && ok2 {
			combinedResults = append(combinedResults, map[string]any{"chunk": chunk, "score": score, "type": "vector"})
		}
	}

	// Normalize scores
	combinedResults, err = normalizeScores(combinedResults)
	if err != nil {
		return nil, fmt.Errorf("failed to normalize scores: %w", err)
	}

	// Sort combined results by score in descending order
	sort.Slice(combinedResults, func(i, j int) bool {
		return combinedResults[i]["score"].(float64) > combinedResults[j]["score"].(float64)
	})

	// Get top 5 chunks
	var chunks []string
	for i := 0; i < len(combinedResults) && i < 5; i++ {
		chunks = append(chunks, combinedResults[i]["chunk"].(string))
	}

	return chunks, nil
}

func CreatePaperIndexesInNeo4j(arxiv_id string) error {
	ftsIndexName := fmt.Sprintf("`%sFTS`", arxiv_id)
	escapedName := fmt.Sprintf("`%s`", arxiv_id)
	var err error
	_, err = neo4j.ExecuteQuery(
		connectionName,
		fmt.Sprintf(
			`
			CREATE FULLTEXT INDEX %s IF NOT EXISTS FOR (n:%s) ON EACH [n.text]
			`,
			ftsIndexName,
			escapedName,
		),
		map[string]any{},
	)
	if err != nil {
		return fmt.Errorf("failed to create FTS index on %s: %w", arxiv_id, err)
	}

	vectorIndexName := fmt.Sprintf("`%sVectorIndex`", arxiv_id)
	_, err = neo4j.ExecuteQuery(
		connectionName,
		fmt.Sprintf(
			`
			CREATE VECTOR INDEX %s IF NOT EXISTS FOR (n:%s) ON (n.embedding) OPTIONS { indexConfig: { `+"`vector.dimensions`: 768, `vector.similarity_function`: 'cosine' } }"+`
			`,
			vectorIndexName,
			escapedName,
		),
		map[string]any{},
	)
	if err != nil {
		return fmt.Errorf("failed to create vector index on %s: %w", arxiv_id, err)
	}
	return nil
}

func InsertPapersIntoNeo4j() error {
	for _, paper := range papers {
		// Get Arxiv ID from the paper link
		arxivID := strings.Split(paper.Link, "/")[4]
		// Check if the paper already exists in neo4j
		exists, err := indexExists("neo4j", arxivID)
		if err != nil {
			return fmt.Errorf("failed to check if paper exists in neo4j: %w", err)
		}
		if exists {
			fmt.Printf("Paper %s already exists in neo4j, skipping\n", arxivID)
			continue
		}
		// Create indexes for the paper
		if err := CreatePaperIndexesInNeo4j(arxivID); err != nil {
			return fmt.Errorf("failed to create indexes for paper %s: %w", arxivID, err)
		}
		// Use python endpoint to get chunks of each paper
		requestBody, err := json.Marshal(map[string]string{
			"link": paper.Link,
		})
		if err != nil {
			return fmt.Errorf("failed to marshal request body for getting chunks: %w", err)
		}
		req := h.NewRequest("http://localhost:3000/get-chunks")
		req.Method = "POST"
		req.Body = requestBody
		req.Headers = h.NewHeaders(map[string][]string{
			"Content-Type": {"application/json"},
		})
		resp, err := h.Fetch(req)
		if err != nil {
			return fmt.Errorf("http fetch failed: %w", err)
		}
		if !resp.Ok() {
			body, _ := io.ReadAll(bytes.NewReader(resp.Body))
			return fmt.Errorf("http fetch failed with status %d : %s, body: %s", resp.Status, resp.StatusText, string(body))
		}
		/*
		Sample response:
		{
			"chunks": [
				{
					"text": "This is the first chunk of the paper...",
					"page_number": 1
				},
				{
					"text": "This is the second chunk of the paper...",
					"page_number": 2
				},
				...
			]
		}
		*/
		var chunks map[string][]map[string]any
		if err := json.NewDecoder(bytes.NewReader(resp.Body)).Decode(&chunks); err != nil {
			return fmt.Errorf("failed to decode json response body: %w", err)
		}
		// Embed the paper text for each chunk and insert into neo4j
		for i, chunk := range chunks["chunks"] {
			embedding, err := PromptEmbedModel(chunk["text"].(string))
			if err != nil {
				return fmt.Errorf("failed to embed chunk: %w", err)
			}
			// Insert the chunk into neo4j
			_, err = neo4j.ExecuteQuery(
				connectionName,
				fmt.Sprintf(
					`
					CREATE (p:%s {name: $name, text: $text, embedding: $embedding})
					`,
					arxivID,
				),
				map[string]any{
					"name":      fmt.Sprintf("%s_%d_%d", arxivID, chunk["page_number"], i),
					"text":      chunk["text"],
					"embedding": embedding,
				},
			)
			if err != nil {
				return fmt.Errorf("failed to insert chunk into neo4j: %w", err)
			}
		}
	}	
	papers = []ArxivResult{}
	return nil	
}

func ClearHistory() {
	messages = []openai.Message {
		openai.NewSystemMessage(systemMessage),
	}
}

func QueryAgent(query string) (string, error) {
	// Get response from the language model
	messages = append(messages, openai.NewUserMessage(query))
	response, err := PromptLLM()
	if err != nil {
		return "", fmt.Errorf("failed to prompt language model: %w", err)
	}
	var nextMessage openai.Message
	// Check if the response is a tool call
	for {
		if !strings.Contains(response, "json") {
			break
		}
	
		// Parse the tool call as JSON from the response in markdown format
		toolCall := strings.Split(response, "```json")[1]
		toolCall = strings.Split(toolCall, "```")[0]
	
		// Execute the tool call
		var toolCallMap map[string]any
		if err := json.Unmarshal([]byte(toolCall), &toolCallMap); err != nil {
			return "", fmt.Errorf("failed to unmarshal tool call: %w", err)
		}
	
		tool, ok := toolCallMap["tool"].(string)
		if !ok {
			return "", fmt.Errorf("failed to get tool from tool call")
		}
	
		params, ok := toolCallMap["params"].(map[string]any)
		if !ok {
			return "", fmt.Errorf("failed to get params from tool call")
		}
	
		switch tool {
		case "SearchDDG":
			query, ok := params["query"].(string)
			if !ok {
				return "", fmt.Errorf("failed to get query from params")
			}
			maxResults, ok := params["maxResults"].(float64)
			if !ok {
				return "", fmt.Errorf("failed to get maxResults from params")
			}
			ddgResponse, err := SearchDDG(query, int(maxResults))
			if err != nil {
				return "", fmt.Errorf("failed to search ddg: %w", err)
			}
			nextMessage = openai.NewAssistantMessage(fmt.Sprintf("I found these results on DuckDuckGo:\n\n%s", ddgResponse))
		case "SearchArxiv":
			query, ok := params["query"].(string)
			if !ok {
				return "", fmt.Errorf("failed to get query from params")
			}
			maxResults, ok := params["maxResults"].(float64)
			if !ok {
				return "", fmt.Errorf("failed to get maxResults from params")
			}
			arxivResponse, err := SearchArxiv(query, int(maxResults))
			if err != nil {
				return "", fmt.Errorf("failed to search arxiv: %w", err)
			}
			nextMessage = openai.NewAssistantMessage(fmt.Sprintf("I found these papers on Arxiv:\n\n%s", arxivResponse))
		case "retrieveBestCategories":
			query, ok := params["query"].(string)
			if (!ok) {
				return "", fmt.Errorf("failed to get query from params")
			}
			categories, err := RetrieveBestCategories(query)
			if err != nil {
				return "", fmt.Errorf("failed to retrieve best categories: %w", err)
			}
			nextMessage = openai.NewAssistantMessage(fmt.Sprintf("Based on your query, the best categories to search for would be %s", categories))
		case "RetrieveBestPapers":
			query, ok := params["query"].(string)
			if !ok {
				return "", fmt.Errorf("failed to get query from params")
			}
			category, ok := params["category"].(string)
			if !ok {
				return "", fmt.Errorf("failed to get category from params")
			}
			papers, err := RetrieveBestPapers(query, category)
			if err != nil {
				return "", fmt.Errorf("failed to retrieve best papers: %w", err)
			}
			nextMessage = openai.NewAssistantMessage(fmt.Sprintf("The best papers on %s in the category %s are %s", query, category, papers))
		case "RetrieveBestChunks":
			query, ok := params["query"].(string)
			if !ok {
				return "", fmt.Errorf("failed to get query from params")
			}
			paperName, ok := params["paperName"].(string)
			if !ok {
				return "", fmt.Errorf("failed to get paperName from params")
			}
			chunks, err := RetrieveBestChunks(query, paperName)
			if err != nil {
				return "", fmt.Errorf("failed to retrieve best chunks: %w", err)
			}
			nextMessage = openai.NewAssistantMessage(fmt.Sprintf("Here's a summary of the most relevant chunks from the paper %s:\n\n%s", paperName, chunks))
		case "InsertPapersIntoNeo4j":
			err := InsertPapersIntoNeo4j()
			if err != nil {
				return "", fmt.Errorf("failed to insert papers into neo4j: %w", err)
			}
			nextMessage = openai.NewAssistantMessage("Okay, I've added the papers and their chunks to the Neo4j database.")
		}
		messages = append(messages, nextMessage)
	
		// Call promptLLM again to get the next response
		response, err = PromptLLM()
		if err != nil {
			return "", fmt.Errorf("failed to prompt language model: %w", err)
		}
	}
	nextMessage = openai.NewAssistantMessage(response)
	messages = append(messages, nextMessage)
	return response, nil
}