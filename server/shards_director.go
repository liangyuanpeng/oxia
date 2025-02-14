// Copyright 2023 StreamNative, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package server

import (
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"go.uber.org/multierr"
	"google.golang.org/grpc/status"
	"io"
	"oxia/common"
	"oxia/common/metrics"
	"oxia/proto"
	"oxia/server/kv"
	"oxia/server/wal"
	"sync"
)

type ShardsDirector interface {
	io.Closer

	GetLeader(shardId int64) (LeaderController, error)
	GetFollower(shardId int64) (FollowerController, error)

	GetOrCreateLeader(namespace string, shardId int64) (LeaderController, error)
	GetOrCreateFollower(namespace string, shardId int64) (FollowerController, error)

	DeleteShard(req *proto.DeleteShardRequest) (*proto.DeleteShardResponse, error)
}

type shardsDirector struct {
	sync.RWMutex

	config    Config
	leaders   map[int64]LeaderController
	followers map[int64]FollowerController

	kvFactory              kv.KVFactory
	walFactory             wal.WalFactory
	replicationRpcProvider ReplicationRpcProvider
	closed                 bool
	log                    zerolog.Logger
}

func NewShardsDirector(config Config, walFactory wal.WalFactory, kvFactory kv.KVFactory, provider ReplicationRpcProvider) ShardsDirector {
	sd := &shardsDirector{
		config:                 config,
		walFactory:             walFactory,
		kvFactory:              kvFactory,
		leaders:                make(map[int64]LeaderController),
		followers:              make(map[int64]FollowerController),
		replicationRpcProvider: provider,
		log: log.With().
			Str("component", "shards-director").
			Logger(),
	}

	metrics.NewGauge("oxia_server_leaders_count", "The number of leader controllers in a server", "count", nil, func() int64 {
		sd.RLock()
		defer sd.RUnlock()
		return int64(len(sd.leaders))
	})
	metrics.NewGauge("oxia_server_followers_count", "The number of followers controllers in a server", "count", nil, func() int64 {
		sd.RLock()
		defer sd.RUnlock()
		return int64(len(sd.followers))
	})

	return sd
}

func (s *shardsDirector) GetLeader(shardId int64) (LeaderController, error) {
	s.RLock()
	defer s.RUnlock()

	if s.closed {
		return nil, common.ErrorAlreadyClosed
	}

	if leader, ok := s.leaders[shardId]; ok {
		// There is already a leader controller for this shard
		return leader, nil
	}

	s.log.Debug().
		Int64("shard", shardId).
		Msg("This node is not hosting shard")
	return nil, status.Errorf(common.CodeNodeIsNotLeader, "node is not leader for shard %d", shardId)
}

func (s *shardsDirector) GetFollower(shardId int64) (FollowerController, error) {
	s.RLock()
	defer s.RUnlock()

	if s.closed {
		return nil, common.ErrorAlreadyClosed
	}

	if follower, ok := s.followers[shardId]; ok {
		// There is already a follower controller for this shard
		return follower, nil
	}

	s.log.Debug().
		Int64("shard", shardId).
		Msg("This node is not hosting shard")
	return nil, status.Errorf(common.CodeNodeIsNotFollower, "node is not follower for shard %d", shardId)
}

func (s *shardsDirector) GetOrCreateLeader(namespace string, shardId int64) (LeaderController, error) {
	s.Lock()
	defer s.Unlock()

	if s.closed {
		return nil, common.ErrorAlreadyClosed
	}

	if leader, ok := s.leaders[shardId]; ok {
		// There is already a leader controller for this shard
		return leader, nil
	} else if follower, ok := s.followers[shardId]; ok {
		// There is an existing follower controller
		// Let's close it and before creating the leader controller

		if err := follower.Close(); err != nil {
			return nil, err
		}

		delete(s.followers, shardId)
	}

	// Create new leader controller
	if lc, err := NewLeaderController(s.config, namespace, shardId, s.replicationRpcProvider, s.walFactory, s.kvFactory); err != nil {
		return nil, err
	} else {
		s.leaders[shardId] = lc
		return lc, nil
	}
}

func (s *shardsDirector) GetOrCreateFollower(namespace string, shardId int64) (FollowerController, error) {
	s.Lock()
	defer s.Unlock()

	if s.closed {
		return nil, common.ErrorAlreadyClosed
	}

	if follower, ok := s.followers[shardId]; ok {
		// There is already a follower controller for this shard
		return follower, nil
	} else if leader, ok := s.leaders[shardId]; ok {
		// There is an existing leader controller
		// Let's close it before creating the follower controller

		if err := leader.Close(); err != nil {
			return nil, err
		}

		delete(s.leaders, shardId)
	}

	// Create new follower controller
	if fc, err := NewFollowerController(s.config, namespace, shardId, s.walFactory, s.kvFactory); err != nil {
		return nil, err
	} else {
		s.followers[shardId] = fc
		return fc, nil
	}
}

func (s *shardsDirector) DeleteShard(req *proto.DeleteShardRequest) (*proto.DeleteShardResponse, error) {
	s.Lock()
	defer s.Unlock()

	if leader, ok := s.leaders[req.ShardId]; ok {
		resp, err := leader.DeleteShard(req)
		if err != nil {
			return nil, err
		}

		delete(s.leaders, req.ShardId)
		return resp, nil
	}

	if follower, ok := s.followers[req.ShardId]; ok {
		resp, err := follower.DeleteShard(req)
		if err != nil {
			return nil, err
		}

		delete(s.followers, req.ShardId)
		return resp, nil
	} else if fc, err := NewFollowerController(s.config, req.Namespace, req.ShardId, s.walFactory, s.kvFactory); err != nil {
		return nil, err
	} else {
		return fc.DeleteShard(req)
	}
}

func (s *shardsDirector) Close() error {
	s.Lock()
	defer s.Unlock()

	s.closed = true
	var err error

	for _, leader := range s.leaders {
		err = multierr.Append(err, leader.Close())
	}

	for _, follower := range s.followers {
		err = multierr.Append(err, follower.Close())
	}

	return err
}
