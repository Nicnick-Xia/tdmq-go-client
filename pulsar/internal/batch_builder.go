// Licensed to the Apache Software Foundation (ASF) under one
// or more contributor license agreements.  See the NOTICE file
// distributed with this work for additional information
// regarding copyright ownership.  The ASF licenses this file
// to you under the Apache License, Version 2.0 (the
// "License"); you may not use this file except in compliance
// with the License.  You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package internal

import (
	"time"

	"github.com/TencentCloud/tdmq-go-client/pulsar/internal/compression"

	pb "github.com/TencentCloud/tdmq-go-client/pulsar/internal/pulsar_proto"
	"github.com/gogo/protobuf/proto"

	log "github.com/sirupsen/logrus"
)

const (
	// DefaultMaxBatchSize init default for maximum number of bytes per batch
	DefaultMaxBatchSize = 128 * 1024

	// DefaultMaxMessagesPerBatch init default num of entries in per batch.
	DefaultMaxMessagesPerBatch = 1000
)

type BuffersPool interface {
	GetBuffer() Buffer
}

// BatchBuilder wraps the objects needed to build a batch.
type BatchBuilder struct {
	buffer Buffer

	// Current number of messages in the batch
	numMessages uint

	// Max number of message allowed in the batch
	maxMessages uint

	// The largest size for a batch sent from this praticular producer.
	// This is used as a baseline to allocate a new buffer that can hold the entire batch
	// without needing costly re-allocations.
	maxBatchSize uint

	producerName string
	producerID   uint64

	cmdSend     *pb.BaseCommand
	msgMetadata *pb.MessageMetadata
	callbacks   []interface{}

	compressionProvider compression.Provider
	buffersPool         BuffersPool
}

// NewBatchBuilder init batch builder and return BatchBuilder pointer. Build a new batch message container.
func NewBatchBuilder(maxMessages uint, maxBatchSize uint, producerName string, producerID uint64,
	compressionType pb.CompressionType, level compression.Level,
	bufferPool BuffersPool) (*BatchBuilder, error) {
	if maxMessages == 0 {
		maxMessages = DefaultMaxMessagesPerBatch
	}
	if maxBatchSize == 0 {
		maxBatchSize = DefaultMaxBatchSize
	}
	bb := &BatchBuilder{
		buffer:       NewBuffer(4096),
		numMessages:  0,
		maxMessages:  maxMessages,
		maxBatchSize: maxBatchSize,
		producerName: producerName,
		producerID:   producerID,
		cmdSend: baseCommand(pb.BaseCommand_SEND,
			&pb.CommandSend{
				ProducerId: &producerID,
			}),
		msgMetadata: &pb.MessageMetadata{
			ProducerName: &producerName,
		},
		callbacks:           []interface{}{},
		compressionProvider: getCompressionProvider(compressionType, level),
		buffersPool:         bufferPool,
	}

	if compressionType != pb.CompressionType_NONE {
		bb.msgMetadata.Compression = &compressionType
	}

	return bb, nil
}

// IsFull check if the size in the current batch exceeds the maximum size allowed by the batch
func (bb *BatchBuilder) IsFull() bool {
	return bb.numMessages >= bb.maxMessages || bb.buffer.ReadableBytes() > uint32(bb.maxBatchSize)
}

func (bb *BatchBuilder) hasSpace(payload []byte) bool {
	msgSize := uint32(len(payload))
	return bb.numMessages > 0 && (bb.buffer.ReadableBytes()+msgSize) > uint32(bb.maxBatchSize)
}

// Add will add single message to batch.
func (bb *BatchBuilder) Add(metadata *pb.SingleMessageMetadata, sequenceID uint64, payload []byte,
	callback interface{}, replicateTo []string, deliverAt time.Time) bool {
	if replicateTo != nil && bb.numMessages != 0 {
		// If the current batch is not empty and we're trying to set the replication clusters,
		// then we need to force the current batch to flush and send the message individually
		return false
	} else if bb.msgMetadata.ReplicateTo != nil {
		// There's already a message with cluster replication list. need to flush before next
		// message can be sent
		return false
	} else if bb.hasSpace(payload) {
		// The current batch is full. Producer has to call Flush() to
		return false
	}

	if bb.numMessages == 0 {
		bb.msgMetadata.SequenceId = proto.Uint64(sequenceID)
		bb.msgMetadata.PublishTime = proto.Uint64(TimestampMillis(time.Now()))
		bb.msgMetadata.SequenceId = proto.Uint64(sequenceID)
		bb.msgMetadata.ProducerName = &bb.producerName
		bb.msgMetadata.ReplicateTo = replicateTo
		bb.msgMetadata.PartitionKey = metadata.PartitionKey

		//For Tencent TDMQ tag ,use the tag message,should close batch send
		bb.msgMetadata.Properties = metadata.Properties

		if deliverAt.UnixNano() > 0 {
			bb.msgMetadata.DeliverAtTime = proto.Int64(int64(TimestampMillis(deliverAt)))
		}

		bb.cmdSend.Send.SequenceId = proto.Uint64(sequenceID)
	}
	addSingleMessageToBatch(bb.buffer, metadata, payload)

	bb.numMessages++
	bb.callbacks = append(bb.callbacks, callback)
	return true
}

func (bb *BatchBuilder) reset() {
	bb.numMessages = 0
	bb.buffer.Clear()
	bb.callbacks = []interface{}{}
	bb.msgMetadata.ReplicateTo = nil
}

// Flush all the messages buffered in the client and wait until all messages have been successfully persisted.
func (bb *BatchBuilder) Flush() (batchData Buffer, sequenceID uint64, callbacks []interface{}) {
	if bb.numMessages == 0 {
		// No-Op for empty batch
		return nil, 0, nil
	}
	log.Debug("BatchBuilder flush: messages: ", bb.numMessages)

	bb.msgMetadata.NumMessagesInBatch = proto.Int32(int32(bb.numMessages))
	bb.cmdSend.Send.NumMessages = proto.Int32(int32(bb.numMessages))

	uncompressedSize := bb.buffer.ReadableBytes()
	bb.msgMetadata.UncompressedSize = &uncompressedSize

	buffer := bb.buffersPool.GetBuffer()
	if buffer == nil {
		buffer = NewBuffer(int(uncompressedSize * 3 / 2))
	}
	serializeBatch(buffer, bb.cmdSend, bb.msgMetadata, bb.buffer, bb.compressionProvider)

	callbacks = bb.callbacks
	sequenceID = bb.cmdSend.Send.GetSequenceId()
	bb.reset()
	return buffer, sequenceID, callbacks
}

func (bb *BatchBuilder) Close() error {
	return bb.compressionProvider.Close()
}

func getCompressionProvider(compressionType pb.CompressionType,
	level compression.Level) compression.Provider {
	switch compressionType {
	case pb.CompressionType_NONE:
		return compression.NewNoopProvider()
	case pb.CompressionType_LZ4:
		return compression.NewLz4Provider()
	case pb.CompressionType_ZLIB:
		return compression.NewZLibProvider()
	case pb.CompressionType_ZSTD:
		return compression.NewZStdProvider(level)
	default:
		log.Panic("unsupported compression type")
		return nil
	}
}
