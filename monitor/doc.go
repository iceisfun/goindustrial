// Package monitor provides a polling engine that periodically reads PLC data
// points over industrial protocols (such as Modbus TCP or EtherNet/IP) and
// detects value changes.
//
// A [Monitor] manages one or more subscriptions, each polling a single
// [plc.DataPoint] at a configurable interval. Poll results are broadcast as
// [Event] values to all registered [Subscriber] instances and to the shared
// event channel returned by [Monitor.Events].
//
// # Change detection
//
// Attach a [ChangeDetector] to a subscription to have the monitor compare
// successive reads and set [Event.Changed] accordingly. The built-in
// [ByteChangeDetector] compares raw bytes and is suitable for most use cases.
//
// # Adaptive read clustering
//
// Wrap your [plc.Reader] in a [ClusteringReader] to coalesce nearby register
// or coil addresses into block reads. This dramatically reduces the number of
// network round trips for protocols like Modbus TCP, where each request
// carries overhead. The clustering plan is rebuilt automatically as
// subscriptions are added or removed.
package monitor
