# Go map writes

In this example, writing to a nil map caused a panic. Create the map with make before inserting entries.
When multiple goroutines access a map and at least one writes, protect access with synchronization such as a mutex.
