/*
Copyright (c) 2026 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the
License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific
language governing permissions and limitations under the License.
*/

package kafka

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	kafkago "github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/magiconair/properties"
	"github.com/spf13/pflag"
)

// Tool provides operations for connecting to a Kafka broker.
type Tool interface {
	// Producer creates a new Kafka producer configured from the loaded properties.
	Producer(ctx context.Context) (*Producer, error)

	// Consumer creates a new Kafka consumer configured from the loaded properties. The group parameter specifies
	// the consumer group.
	Consumer(ctx context.Context, group string) (*Consumer, error)
}

// ToolBuilder builds instances of the Kafka tool.
type ToolBuilder struct {
	logger           *slog.Logger
	propertiesValues map[string]any
	propertiesFiles  []string
}

type tool struct {
	logger *slog.Logger
	config kafkago.ConfigMap
}

// NewTool creates a new Kafka tool builder.
func NewTool() *ToolBuilder {
	return &ToolBuilder{}
}

// SetLogger sets the logger for the tool.
func (b *ToolBuilder) SetLogger(value *slog.Logger) *ToolBuilder {
	b.logger = value
	return b
}

// SetProperty sets a property value.
func (b *ToolBuilder) SetProperty(key string, value any) *ToolBuilder {
	b.propertiesValues[key] = value
	return b
}

// AddPropertiesFile adds a path to a file or directory containing Kafka client properties.
func (b *ToolBuilder) AddPropertiesFile(value string) *ToolBuilder {
	b.propertiesFiles = append(b.propertiesFiles, value)
	return b
}

// SetFlags sets the command line flags that should be used to configure the tool. This is optional.
func (b *ToolBuilder) SetFlags(flags *pflag.FlagSet) *ToolBuilder {
	if flags == nil {
		return b
	}
	values, err := flags.GetStringArray(propertiesFileFlagName)
	if err != nil {
		b.logger.Error(
			"Failed to get flag value",
			slog.String("flag", propertiesFileFlagName),
			slog.Any("error", err),
		)
	} else {
		for _, value := range values {
			b.AddPropertiesFile(value)
		}
	}
	return b
}

// Build creates the Kafka tool from the configured parameters.
func (b *ToolBuilder) Build() (result Tool, err error) {
	// Check parameters:
	if b.logger == nil {
		err = errors.New("logger is mandatory")
		return
	}

	// Load properties from a file or a directory of files:
	config := kafkago.ConfigMap{}
	for propertyName, propertyValue := range b.propertiesValues {
		config[propertyName] = propertyValue
	}
	for _, propertiesFile := range b.propertiesFiles {
		err = b.loadProperties(propertiesFile, config)
		if err != nil {
			return
		}
	}
	b.logger.Debug(
		"Loaded Kafka properties",
		slog.Any("!properties", config),
	)

	// Check that the minimum required properties are set:
	if config["bootstrap.servers"] == "" {
		err = errors.New("'bootstrap.servers' property is mandatory")
		return
	}

	// Create the object:
	result = &tool{
		logger: b.logger,
		config: config,
	}
	return
}

// loadProperties loads properties from the given path and merges the results into the given config map. If the path is
// a file it is loaded directly. If the path is a directory all files in it are loaded and merged together.
func (b *ToolBuilder) loadProperties(file string, config kafkago.ConfigMap) error {
	info, err := os.Stat(file)
	if err != nil {
		return fmt.Errorf("failed to check if Kafka properties file '%s' exists: %w", file, err)
	}
	loadFile := func(path string) error {
		values, err := properties.LoadFile(path, properties.UTF8)
		if err != nil {
			return fmt.Errorf("failed to load Kafka properties file '%s': %w", path, err)
		}
		for _, propertyName := range values.Keys() {
			propertyValue, _ := values.Get(propertyName)
			config[propertyName] = propertyValue
		}
		b.logger.Debug(
			"Loaded Kafka properties",
			slog.String("file", path),
			slog.Any("!properties", values),
		)
		return nil
	}
	if !info.IsDir() {
		return loadFile(file)
	}
	entries, err := os.ReadDir(file)
	if err != nil {
		return fmt.Errorf("failed to read Kafka properties directory '%s': %w", file, err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || strings.HasPrefix(name, ".") {
			continue
		}
		path := filepath.Join(file, name)
		err := loadFile(path)
		if err != nil {
			return err
		}
	}
	return nil
}

// Producer creates a new Kafka producer configured from the loaded properties.
func (t *tool) Producer(ctx context.Context) (result *Producer, err error) {
	logger := t.logger.With()
	logger.InfoContext(
		ctx,
		"Creating Kafka producer",
	)
	config, err := t.cloneConfig()
	if err != nil {
		return
	}
	producer, err := kafkago.NewProducer(config)
	if err != nil {
		return
	}
	result = &Producer{
		logger:   logger,
		producer: producer,
	}
	return
}

// Consumer creates a new Kafka consumer configured from the loaded properties.
func (t *tool) Consumer(ctx context.Context, group string) (result *Consumer, err error) {
	logger := t.logger.With(
		slog.String("group", group),
	)
	logger.InfoContext(
		ctx,
		"Creating Kafka consumer",
	)
	config, err := t.cloneConfig()
	if err != nil {
		return
	}
	err = config.SetKey("group.id", group)
	if err != nil {
		return
	}
	value, err := config.Get("auto.offset.reset", nil)
	if err != nil {
		return
	}
	if value == nil {
		err = config.SetKey("auto.offset.reset", "earliest")
		if err != nil {
			return
		}
	}
	consumer, err := kafkago.NewConsumer(config)
	if err != nil {
		return
	}
	result = &Consumer{
		logger:   logger,
		consumer: consumer,
	}
	return
}

// cloneConfig creates a new configuration from the loaded properties, so that it can be modified, for example to set
// the additional properties required by consumers or producers, without affecting the original one.
func (t *tool) cloneConfig() (result *kafkago.ConfigMap, err error) {
	clone := &kafkago.ConfigMap{}
	for key, value := range t.config {
		err := clone.SetKey(key, value)
		if err != nil {
			return nil, fmt.Errorf("failed to set '%s' property: %w", key, err)
		}
	}
	result = clone
	return
}
