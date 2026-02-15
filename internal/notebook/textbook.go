package notebook

import "path/filepath"

type Index struct {
	path          string   `yaml:"-"`
	isBook        bool     `yaml:"-"`
	Kind          string   `yaml:"kind"`
	ID            string   `yaml:"id"`
	Name          string   `yaml:"name"`
	NotebookPaths []string `yaml:"notebooks"`

	Notebooks [][]StoryNotebook `yaml:"-"`
}

func (index Index) IsBook() bool {
	return index.isBook
}

func (index Index) GetNotebookPath(i int) string {
	path := index.NotebookPaths[i]
	return filepath.Join(index.path, path)
}
