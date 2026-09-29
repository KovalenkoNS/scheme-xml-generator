// Обход вложенного дерева OBJTYPE для загрузки и индексации библиотеки.
package library

// walkObjectTypes обходит типы объектов во всех разделах и вложенных узлах XML-библиотеки.
// Передаёт указатель каждого OBJTYPE заданному посетителю для индексации типов/шаблонов.
func walkObjectTypes(document *Document, visit func(*ObjectType)) {
	var walk func([]ObjectType)
	// Посещает текущий уровень типов и затем вложенные OBJTYPE.
	// Передаёт исходные узлы visitor без копирования или изменения XML-модели.
	walk = func(objects []ObjectType) {
		for i := range objects {
			object := &objects[i]
			visit(object)
			walk(object.Child.ObjectTypes)
		}
	}
	for i := range document.Sections {
		walk(document.Sections[i].Other.ObjectTypes)
	}
}
