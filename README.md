# Research Assistant

## Description
This project is a research assistant that helps researchers to find relevant papers based on their research interests and then query the papers for specific information. The project has been mainly built using modus, Go, Python and Neo4j.

For more information, please refer to the [this submission](https://devpost.com/software/researchassistant).

## Getting Started
### `research-assistant` directory
1. Install the dependencies by running `go mod tidy`.
2. Set the environment variables in the `.env.dev.local` file.
3. Run the application by executing `modus dev`.

### `tools-backend` directory
1. Install the dependencies by running `pip install -r requirements.txt`.
2. Set your parameters in `init_graph_store.py` to initialize the graph store in Neo4j.
3. Run the application by executing `python main.py`.