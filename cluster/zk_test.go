package cluster

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/samuel/go-zookeeper/zk"
)

var _ = Describe("ZK", func() {
	var subject *ZK
	var err error
	var servers = []string{"localhost:22181"}

	BeforeEach(func() {
		subject, err = NewZK(servers, 1e9)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if subject != nil {
			Expect(subject.DeleteAll("/consumers/" + t_GROUP)).To(BeNil())
			subject.Close()
		}
	})

	It("should connect to ZooKeeper", func() {
		Expect(subject).To(BeAssignableToTypeOf(&ZK{}))
	})

	Describe("high-level API", func() {

		var checkOwner = func(num string) string {
			val, _, _ := subject.Get("/consumers/" + t_GROUP + "/owners/" + t_TOPIC + "/" + num)
			return string(val)
		}

		It("should return consumers within a group", func() {
			Expect(subject.RegisterGroup(t_GROUP)).To(BeNil())

			sub, _, err := subject.Consumers(t_GROUP)
			Expect(err).To(BeNil())
			Expect(sub).To(BeEmpty())

			err = subject.Create("/consumers/"+t_GROUP+"/ids/consumer-c", []byte{'C'}, true)
			Expect(err).To(BeNil())
			err = subject.Create("/consumers/"+t_GROUP+"/ids/consumer-a", []byte{'A'}, true)
			Expect(err).To(BeNil())
			err = subject.Create("/consumers/"+t_GROUP+"/ids/consumer-b", []byte{'B'}, true)
			Expect(err).To(BeNil())

			sub, _, err = subject.Consumers(t_GROUP)
			Expect(err).To(BeNil())
			Expect(sub).To(Equal([]string{"consumer-a", "consumer-b", "consumer-c"}))
		})

		It("should register groups", func() {
			ok, err := subject.Exists("/consumers/" + t_GROUP + "/ids")
			Expect(err).To(BeNil())
			Expect(ok).To(BeFalse())

			err = subject.RegisterGroup(t_GROUP)
			Expect(err).To(BeNil())

			ok, err = subject.Exists("/consumers/" + t_GROUP + "/ids")
			Expect(err).To(BeNil())
			Expect(ok).To(BeTrue())
		})

		It("should register consumers (ephemeral) ", func() {
			Expect(subject.RegisterGroup(t_GROUP)).To(BeNil())

			other, err := NewZK(servers, 1e9)
			Expect(err).To(BeNil())

			strs, _, err := subject.Consumers(t_GROUP)
			Expect(err).To(BeNil())
			Expect(strs).To(BeEmpty())

			err = subject.RegisterConsumer(t_GROUP, "consumer-b", "topic")
			Expect(err).To(BeNil())
			err = other.RegisterConsumer(t_GROUP, "consumer-a", "topic")
			Expect(err).To(BeNil())
			err = subject.RegisterConsumer(t_GROUP, "consumer-b", "topic")
			Expect(err).To(Equal(zk.ErrNodeExists))

			strs, _, err = subject.Consumers(t_GROUP)
			Expect(err).To(BeNil())
			Expect(strs).To(Equal([]string{"consumer-a", "consumer-b"}))

			other.Close()
			strs, _, err = subject.Consumers(t_GROUP)
			Expect(err).To(BeNil())
			Expect(strs).To(Equal([]string{"consumer-b"}))

			val, _, err := subject.Get("/consumers/" + t_GROUP + "/ids/consumer-b")
			Expect(err).To(BeNil())
			Expect(string(val)).To(ContainSubstring(`"subscription":{"topic":1}`))
		})

		It("should claim partitions (ephemeral)", func() {
			Expect(subject.Claim(t_GROUP, t_TOPIC, 0, "consumer-a")).To(BeNil())
			Expect(checkOwner("0")).To(Equal(`consumer-a`))
		})

		It("should wait with claim until available", func() {
			Expect(subject.Claim(t_GROUP, t_TOPIC, 1, "consumer-b")).To(BeNil())
			go func() {
				subject.Claim(t_GROUP, t_TOPIC, 1, "consumer-c")
			}()
			Expect(checkOwner("1")).To(Equal(`consumer-b`))
			Expect(subject.Release(t_GROUP, t_TOPIC, 1, "consumer-b")).To(BeNil())
			Eventually(func() string { return checkOwner("1") }).Should(Equal(`consumer-c`))
		})

		It("should release partitions", func() {
			Expect(subject.Release(t_GROUP, t_TOPIC, 0, "consumer-a")).To(BeNil())

			Expect(subject.Claim(t_GROUP, t_TOPIC, 0, "consumer-a")).To(BeNil())
			Expect(subject.Release(t_GROUP, t_TOPIC, 0, "consumer-a")).To(BeNil())

			Expect(subject.Claim(t_GROUP, t_TOPIC, 0, "consumer-a")).To(BeNil())
			Expect(subject.Release(t_GROUP, t_TOPIC, 0, "consumer-b")).To(Equal(zk.ErrNotLocked))
		})

		It("should retrieve offsets", func() {
			offset, err := subject.Offset(t_GROUP, t_TOPIC, 0)
			Expect(err).To(BeNil())
			Expect(offset).To(Equal(int64(0)))

			err = subject.Create("/consumers/"+t_GROUP+"/offsets/"+t_TOPIC+"/0", []byte("14798"), false)
			Expect(err).To(BeNil())

			offset, err = subject.Offset(t_GROUP, t_TOPIC, 0)
			Expect(err).To(BeNil())
			Expect(offset).To(Equal(int64(14798)))
		})

		It("should commit offsets", func() {
			Expect(subject.Commit(t_GROUP, t_TOPIC, 0, 999)).To(BeNil())

			val, stat, err := subject.Get("/consumers/" + t_GROUP + "/offsets/" + t_TOPIC + "/0")
			Expect(err).To(BeNil())
			Expect(string(val)).To(Equal(`999`))
			Expect(stat.Version).To(Equal(int32(0)))

			Expect(subject.Commit(t_GROUP, t_TOPIC, 0, 2999)).To(BeNil())
			offset, err := subject.Offset(t_GROUP, t_TOPIC, 0)
			Expect(err).To(BeNil())
			Expect(offset).To(Equal(int64(2999)))
		})

	})

	Describe("low-level API", func() {

		It("should check path existence", func() {
			ok, err := subject.Exists("/consumers/" + t_GROUP + "/ids")
			Expect(err).To(BeNil())
			Expect(ok).To(BeFalse())
		})

		It("should create dirs recursively", func() {
			ok, _ := subject.Exists("/consumers/" + t_GROUP + "/ids")
			Expect(ok).To(BeFalse())

			err = subject.MkdirAll("/consumers/" + t_GROUP + "/ids")
			Expect(err).To(BeNil())
			err = subject.MkdirAll("/consumers/" + t_GROUP + "/ids")
			Expect(err).To(BeNil())

			ok, _ = subject.Exists("/consumers/" + t_GROUP + "/ids")
			Expect(ok).To(BeTrue())
		})

		It("should create entries", func() {
			err = subject.Create("/consumers/"+t_GROUP+"/ids/x", []byte{'X'}, false)
			Expect(err).To(BeNil())
			err = subject.Create("/consumers/"+t_GROUP+"/ids/x", []byte{'Y'}, false)
			Expect(err).To(Equal(zk.ErrNodeExists))
		})

		It("should create ephemeral entries", func() {
			other, err := NewZK(servers, 1e9)
			Expect(err).To(BeNil())
			err = other.Create("/consumers/"+t_GROUP+"/ids/x", []byte{'X'}, true)
			Expect(err).To(BeNil())

			ok, _ := subject.Exists("/consumers/" + t_GROUP + "/ids/x")
			Expect(ok).To(BeTrue())

			other.Close()
			ok, _ = subject.Exists("/consumers/" + t_GROUP + "/ids/x")
			Expect(ok).To(BeFalse())
		})

	})
})
