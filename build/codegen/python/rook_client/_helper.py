import logging
try:
    from typing import List, Dict, Any, Optional
except ImportError:
    pass

logger = logging.getLogger(__name__)

# Tricking mypy to think `_omit`'s type is NoneType
# To make us not add things like `Union[Optional[str], OmitType]`
NoneType = type(None)
_omit = None  # type: NoneType
_omit = object()  # type: ignore


# Don't add any additionalProperties to objects. Useful for completeness testing
STRICT = False


def _property_from_json(data, breadcrumb, name, py_name, typ, required, nullable):
    if not required and name not in data:
        return _omit
    obj = data[name]
    if nullable and obj is None:
        return obj
    if issubclass(typ, CrdObject) or issubclass(typ, CrdObjectList):
        return typ.from_json(obj, breadcrumb + '.' + name)
    return obj


class CrdObject(object):
    _properties = []  # type: List

    def __init__(self, **kwargs):
        for prop in self._properties:
            setattr(self, prop[1], kwargs.pop(prop[1]))
        if kwargs:
            raise TypeError(
                '{} got unexpected arguments {}'.format(self.__class__.__name__, kwargs.keys()))
        self._additionalProperties = {}  # type: Dict[str, Any]

    def _property_impl(self, name):
        obj = getattr(self, '_' + name)
        if obj is _omit:
            raise AttributeError(name + ' not found')
        return obj

    def _property_to_json(self, name, py_name, typ, required, nullable):
        obj = getattr(self, '_' + py_name)
        if issubclass(typ, CrdObject) or issubclass(typ, CrdObjectList):
            if nullable and obj is None:
                return obj
            if not required and obj is _omit:
                return obj
            return obj.to_json()
        else:
            return obj

    def to_json(self):
        # type: () -> Dict[str, Any]
        res = {p[0]: self._property_to_json(*p) for p in self._properties}
        res.update(self._additionalProperties)
        return {k: v for k, v in res.items() if v is not _omit}

    @classmethod
    def from_json(cls, data, breadcrumb=''):
        try:
            sanitized = {
                p[1]: _property_from_json(data, breadcrumb, *p) for p in cls._properties
            }
            extra = {k:v for k,v in data.items() if k not in sanitized}
            ret = cls(**sanitized)
            ret._additionalProperties = {} if STRICT else extra
            return ret
        except (TypeError, AttributeError, KeyError):
            logger.exception(breadcrumb)
            raise


class CrdClass(CrdObject):
    @classmethod
    def from_json(cls, data, breadcrumb=''):
        kind = data['kind']
        if kind != cls.__name__:
            raise ValueError("kind mismatch: {} != {}".format(kind, cls.__name__))
        return super(CrdClass, cls).from_json(data, breadcrumb)

    def to_json(self):
        ret = super(CrdClass, self).to_json()
        ret['kind'] = self.__class__.__name__
        return ret


class CrdObjectList(list):
    # Py3: Replace `Any` with `TypeVar('T_CrdObject', bound='CrdObject')`
    _items_type = None  # type: Optional[Any]

    def to_json(self):
        # type: () -> List
        if self._items_type is None:
            return self
        return [e.to_json() for e in self]

    @classmethod
    def from_json(cls, data, breadcrumb=''):
        if cls._items_type is None:
            return cls(data)
        return cls(cls._items_type.from_json(e, breadcrumb + '[]') for e in data)
